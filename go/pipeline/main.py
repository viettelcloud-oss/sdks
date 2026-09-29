#!/usr/bin/env python3
"""
main.py — orchestrate the Viettel Cloud Go SDK pipeline.

Reads pipeline.yaml and drives the whole build. The Makefile only forwards to
the commands below; the real work is here and in normalize.py / gen_facade.py.

    python3 pipeline/main.py init                  # go mod init/edit
    python3 pipeline/main.py spec [--fetch]        # fetch + normalize the spec
    python3 pipeline/main.py generate [SERVICE...] # codegen + facades
    python3 pipeline/main.py all [--fetch]         # init + spec + generate
    python3 pipeline/main.py clean                 # remove generated files

Environment overrides: SPEC_URL (spec.url), SERVICES (which services to build).
"""

import argparse
import copy
import json
import os
import re
import subprocess
import sys
import tempfile
from pathlib import Path

try:
    import yaml
except ImportError:
    print("ERROR: PyYAML not found. Install: pip install pyyaml", file=sys.stderr)
    sys.exit(1)

HERE = Path(__file__).resolve().parent          # pipeline/
ROOT = HERE.parent                              # go/
NORMALIZE_PY = str(HERE / "normalize.py")
GEN_FACADE_PY = str(HERE / "gen_facade.py")

# Upstream runtime import that oapi-codegen emits, repointed at our local copy.
UPSTREAM_RUNTIME = '"github.com/oapi-codegen/runtime"'

# Unbounded body read that oapi-codegen emits in every Parse*Response, and the
# size-limited reader from core that replaces it.
UNBOUNDED_BODY_READ = "io.ReadAll(rsp.Body)"
BOUNDED_BODY_READ = "core.ReadResponseBody(rsp)"


def load_dotenv(paths):
    """Load key-value pairs from .env files into os.environ if not already set."""
    for path in paths:
        p = Path(path)
        if not p.is_file():
            continue
        with open(p, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    key, val = line.split("=", 1)
                    key = key.strip()
                    val = val.strip().strip("'\"")
                    if key and key not in os.environ:
                        os.environ[key] = val


def build_url(base_url, path_or_url):
    """Combine base_url and path_or_url if path_or_url is a relative path."""
    if not path_or_url:
        return ""
    if path_or_url.startswith("http://") or path_or_url.startswith("https://"):
        return path_or_url
    if base_url:
        return f"{base_url.rstrip('/')}/{path_or_url.lstrip('/')}"
    return path_or_url


def load_dotenv(paths):
    """Load key-value pairs from .env files into os.environ if not already set."""
    for path in paths:
        p = Path(path)
        if not p.is_file():
            continue
        with open(p, encoding="utf-8") as f:
            for line in f:
                line = line.strip()
                if not line or line.startswith("#"):
                    continue
                if "=" in line:
                    key, val = line.split("=", 1)
                    key = key.strip()
                    val = val.strip().strip("'\"")
                    if key and key not in os.environ:
                        os.environ[key] = val


def build_url(base_url, path_or_url):
    """Combine base_url and path_or_url if path_or_url is a relative path."""
    if not path_or_url:
        return ""
    if path_or_url.startswith("http://") or path_or_url.startswith("https://"):
        return path_or_url
    if base_url:
        return f"{base_url.rstrip('/')}/{path_or_url.lstrip('/')}"
    return path_or_url


def load_config(config_path):
    """Load pipeline.yaml and apply environment overrides."""
    load_dotenv([ROOT / ".env", ROOT.parent / ".env"])
    with open(config_path) as f:
        config = yaml.safe_load(f)
    if os.environ.get("SPEC_URL"):
        config["spec"]["url"] = os.environ["SPEC_URL"]
    if os.environ.get("SPEC_BASE_URL"):
        config.setdefault("spec", {})["base_url"] = os.environ["SPEC_BASE_URL"]
    return config


def deep_merge(base, override):
    """Recursively merge `override` onto a copy of `base` (dicts only)."""
    result = copy.deepcopy(base)
    for key, value in (override or {}).items():
        if isinstance(value, dict) and isinstance(result.get(key), dict):
            result[key] = deep_merge(result[key], value)
        else:
            result[key] = copy.deepcopy(value)
    return result


def run(cmd, quiet=False, **kwargs):
    """Run a subprocess, streaming stderr on failure. Returns True on success."""
    result = subprocess.run(cmd, capture_output=True, text=True, **kwargs)
    if result.returncode != 0:
        sys.stderr.write(result.stdout)
        sys.stderr.write(result.stderr)
        return False
    if not quiet and result.stdout.strip():
        print(result.stdout.rstrip())
    return True


# --------------------------------------------------------------------- init ---
def init_module(config, quiet=False):
    """Ensure go.mod exists with the configured module path and Go version.

    pipeline.yaml owns the module path, so an existing go.mod is rewritten to
    match it as well: the generated imports and go.mod can never disagree.
    """
    path = config["module"]["path"]
    go_version = str(config["module"]["go_version"])
    if not (ROOT / "go.mod").exists():
        if not quiet:
            print(f"go mod init {path}")
        if not run(["go", "mod", "init", path], quiet, cwd=ROOT):
            return False
    return run(["go", "mod", "edit", f"-module={path}", f"-go={go_version}"],
               quiet, cwd=ROOT)


# --------------------------------------------------------------------- spec ---
def fetch_spec(url, dest, quiet=False):
    """Download the OpenAPI spec with curl."""
    if not (url.startswith("http://") or url.startswith("https://")):
        print(
            f"ERROR: cannot fetch spec from relative path '{url}': SPEC_BASE_URL is not configured.\n"
            f"Set SPEC_BASE_URL in .env or via environment variable (e.g. SPEC_BASE_URL=http://<backend-spec-endpoint>)",
            file=sys.stderr,
        )
        return False
    if not quiet:
        print(f"Fetching spec from {url} ...")
    try:
        return run(["curl", "-fsS", url, "-o", dest], quiet, cwd=ROOT)
    except FileNotFoundError:
        print("ERROR: curl not found (needed to fetch spec)", file=sys.stderr)
        return False


def resolve_spec(svc, config):
    """Resolve the (url, src, edited) files a service reads.

    Most services share the default spec. A service with its own `spec:` block
    (typically just a different backend `url` or `path`) gets source and edited
    files named from the service, so a second backend never clobbers the shared spec.
    """
    default = config["spec"]
    base_url = default.get("base_url") or os.environ.get("SPEC_BASE_URL", "")
    default_path_or_url = default.get("url") or default.get("path", "")
    default_url = build_url(base_url, default_path_or_url)

    override = svc.get("spec")
    if not override:
        return dict(url=default_url, src=default["src"],
                    edited=default["edited"])
    name = svc["name"]
    override_url = override.get("url") or override.get("path")
    resolved_url = build_url(base_url, override_url) if override_url else default_url
    return dict(
        url=resolved_url,
        src=override.get("src", f"openapi.{name}.json"),
        edited=override.get("edited", f"openapi_edited.{name}.json"),
    )


def spec_groups(config):
    """Group services by the spec file they read.

    Services on the default spec share one fetch + normalize; each service with
    its own `spec:` forms its own group. Keyed by edited-file path so services
    pointing at the same spec are processed once.
    """
    groups = {}
    for svc in config["services"]:
        spec = resolve_spec(svc, config)
        group = groups.setdefault(spec["edited"], {**spec, "services": []})
        group["services"].append(svc)
    return groups


def effective_policy(config, services):
    """Default normalize policy merged with each service's `policy` extras.

    One normalize pass runs per spec, so only the services reading that spec
    fold their extras into its policy.
    """
    policy = {k: v for k, v in config["normalize"].items()}
    for svc in services:
        if svc.get("policy"):
            policy = deep_merge(policy, svc["policy"])
    return policy


def normalize_spec(src, edited, policy, module_path, quiet=False):
    """Write the policy to a temp file and run normalize.py on one spec."""
    if not quiet:
        print(f"Normalizing {src} -> {edited} ...")

    with tempfile.NamedTemporaryFile(
        "w", suffix=".json", delete=False, dir=ROOT
    ) as pf:
        json.dump(policy, pf)
        policy_path = pf.name
    try:
        cmd = [
            sys.executable, NORMALIZE_PY, src, edited,
            "--module-path", module_path,
            "--policy", policy_path,
        ]
        return run(cmd, quiet, cwd=ROOT)
    finally:
        os.unlink(policy_path)


def normalize_specs(config, fetch=False, quiet=False, require_src=True):
    """Fetch (optionally) and normalize each distinct spec.

    Covers the shared spec plus any a service points at its own backend. With
    require_src False, a group whose raw spec is absent is skipped, so an
    edited spec supplied by hand is still used as is.
    """
    for group in spec_groups(config).values():
        src, edited = group["src"], group["edited"]
        if fetch:
            if not fetch_spec(group["url"], src, quiet):
                return False
        elif not (ROOT / src).exists():
            if not require_src:
                continue
            print(f"ERROR: spec not found: {src} (use --fetch to download)",
                  file=sys.stderr)
            return False
        elif not quiet:
            print(f"Reusing {src} ...")
        policy = effective_policy(config, group["services"])
        if not normalize_spec(src, edited, policy,
                              config["module"]["path"], quiet):
            return False
    return True


# ----------------------------------------------------------------- generate ---
def repoint_runtime_import(gen_file, module_path):
    """Repoint oapi-codegen's runtime import at the local internal/oapi package.

    The generated code calls into it under the alias `runtime`, so only the
    import line changes — no call sites. Errors if the expected line is absent.
    """
    text = gen_file.read_text()
    if UPSTREAM_RUNTIME not in text:
        return  # spec produced no runtime calls; nothing to repoint
    replacement = f'runtime "{module_path}/internal/oapi"'
    new_text = text.replace(UPSTREAM_RUNTIME, replacement)
    gen_file.write_text(new_text)


def bound_response_reads(gen_file, module_path):
    """Replace oapi-codegen's unbounded response reads with core.ReadResponseBody.

    The client transport already limits response size. This second layer keeps
    the limit when a generated parser gets a body from anywhere else, and lets
    a static check forbid io.ReadAll in generated code. Raises if a read was
    left behind, so a changed oapi-codegen template cannot bypass the limit.
    """
    text = gen_file.read_text()
    if UNBOUNDED_BODY_READ not in text:
        return  # spec produced no response parsers; nothing to bound
    text = text.replace(UNBOUNDED_BODY_READ, BOUNDED_BODY_READ)
    if "io.ReadAll(" in text:
        raise RuntimeError(f"{gen_file}: io.ReadAll left after bounding response reads")

    core_import = f'core "{module_path}/core"'
    if core_import not in text:
        text = text.replace("import (\n", f"import (\n\t{core_import}\n", 1)
    if not re.search(r"\bio\.", text):
        text = re.sub(r'\n\t"io"\n', "\n", text, count=1)
    gen_file.write_text(text)


def generate_service(svc, config, quiet=False):
    """Run oapi-codegen, repoint the import, then build the facade for one service."""
    name = svc["name"]
    folder = svc["folder"]
    tag = svc["tag"]
    module_path = config["module"]["path"]
    edited = resolve_spec(svc, config)["edited"]

    if not quiet:
        print(f"Generating {name} (tag: {tag}) ...")

    gen_dir = ROOT / folder / "internal" / "gen"
    gen_dir.mkdir(parents=True, exist_ok=True)
    gen_file = gen_dir / f"{name}.gen.go"

    # Effective oapi-codegen config: defaults + per-service override + injected
    # output path and tag filter.
    oapi_cfg = deep_merge(config["oapi_codegen"]["config"], svc.get("config"))
    oapi_cfg["output"] = f"{folder}/internal/gen/{name}.gen.go"
    oapi_cfg.setdefault("output-options", {})["include-tags"] = [tag]

    cfg_path = gen_dir / "oapi.cfg.yaml"
    cfg_path.write_text(yaml.safe_dump(oapi_cfg, sort_keys=False))

    version = config["oapi_codegen"]["version"]
    codegen = (
        f"github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@{version}"
    )
    if not run(
        ["go", "run", codegen, "--config", str(cfg_path), edited],
        quiet, cwd=ROOT,
    ):
        print(f"ERROR: oapi-codegen failed for {name}", file=sys.stderr)
        return False

    repoint_runtime_import(gen_file, module_path)
    bound_response_reads(gen_file, module_path)

    # Facade (client.go, operations.go, types.go, error.go, doc.go, README.md).
    if not run(
        [sys.executable, GEN_FACADE_PY, edited, folder, tag,
         "--module-path", module_path],
        quiet, cwd=ROOT,
    ):
        print(f"ERROR: facade generation failed for {name}", file=sys.stderr)
        return False

    # Format everything we just wrote.
    go_files = [str(p) for p in (ROOT / folder).rglob("*.go")]
    if go_files:
        run(["gofmt", "-w", *go_files], quiet, cwd=ROOT)
    return True


def select_services(config, names):
    """Resolve requested service names (or SERVICES env) to config entries."""
    if not names:
        env = os.environ.get("SERVICES", "").split()
        names = env or None
    if not names:
        return config["services"]
    by_name = {s["name"]: s for s in config["services"]}
    selected = []
    for n in names:
        if n not in by_name:
            print(f"ERROR: unknown service '{n}' (not in pipeline.yaml)",
                  file=sys.stderr)
            return None
        selected.append(by_name[n])
    return selected


def generate_code(config, names, quiet=False):
    services = select_services(config, names)
    if services is None:
        return False
    for svc in services:
        edited = resolve_spec(svc, config)["edited"]
        if not (ROOT / edited).exists():
            print(f"ERROR: normalized spec not found: {edited} "
                  f"(run 'spec' first)", file=sys.stderr)
            return False
        if not generate_service(svc, config, quiet):
            return False
    return True


# -------------------------------------------------------------------- clean ---
def clean(config, quiet=False):
    # doc.go and README.md are hand-maintained, so clean leaves them alone.
    for svc in config["services"]:
        folder = ROOT / svc["folder"]
        subprocess.run(["rm", "-rf", str(folder / "internal")])
        for f in ("client.go", "error.go", "operations.go", "types.go"):
            (folder / f).unlink(missing_ok=True)
    for group in spec_groups(config).values():
        (ROOT / group["edited"]).unlink(missing_ok=True)
    if not quiet:
        print("Cleaned generated files.")
    return True


# --------------------------------------------------------------------- main ---
def main():
    parser = argparse.ArgumentParser(
        description="Viettel Cloud Go SDK pipeline orchestrator",
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "command",
        choices=["init", "spec", "generate", "all", "clean"],
        help="init | spec (fetch+normalize) | generate | all | clean",
    )
    parser.add_argument("services", nargs="*",
                        help="services for 'generate' (default: all / $SERVICES)")
    parser.add_argument("--fetch", action="store_true",
                        help="fetch the spec from the backend (default: reuse)")
    parser.add_argument("--config", default=str(ROOT / "pipeline.yaml"),
                        help="config file (default: pipeline.yaml)")
    parser.add_argument("-q", "--quiet", action="store_true",
                        help="suppress progress output")
    args = parser.parse_args()

    try:
        config = load_config(args.config)
    except FileNotFoundError:
        print(f"ERROR: config file not found: {args.config}", file=sys.stderr)
        return 1
    except yaml.YAMLError as e:
        print(f"ERROR: invalid YAML in {args.config}: {e}", file=sys.stderr)
        return 1

    if args.command == "clean":
        return 0 if clean(config, args.quiet) else 1

    # Every build command needs the module in place first.
    if args.command in ("init", "spec", "generate", "all"):
        if not init_module(config, args.quiet):
            return 1
    if args.command == "init":
        return 0

    if args.command in ("spec", "all"):
        if not normalize_specs(config, args.fetch, args.quiet):
            return 1
    elif args.command == "generate":
        # The normalized spec embeds the module path and the normalize policy,
        # so refresh it from the raw spec: an edited spec left over from an
        # older pipeline.yaml would otherwise generate stale imports.
        if not normalize_specs(config, quiet=args.quiet, require_src=False):
            return 1

    if args.command in ("generate", "all"):
        if not generate_code(config, args.services, args.quiet):
            return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
