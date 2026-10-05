#!/usr/bin/env python3
"""
gen_facade.py — Generate the public SDK facade from the OpenAPI spec.

Reads openapi_edited.json and produces:
  <svc>/client.go        — Client struct, NewClient, functional options
  <svc>/types.go         — type aliases re-exporting generated models
  <svc>/operations.go    — typed facade methods wrapping internal/gen
  <svc>/error.go         — re-exports sentinels from core

doc.go and README.md are hand-maintained and never generated.

Usage (called by main.py, which supplies the tag from pipeline.yaml):
    python3 gen_facade.py <spec_json> <svc_dir> <tag> --module-path MODULE
"""

import argparse
import json
import re
from pathlib import Path

# ---------------------------------------------------------------------------
# Naming helpers
# ---------------------------------------------------------------------------

_ABBREVS = {'id': 'ID', 'url': 'URL', 'http': 'HTTP', 'ip': 'IP',
            'vpc': 'VPC', 'qos': 'QoS', 'os': 'OS'}


def pascal(s):
    return ''.join(w.capitalize() for w in re.split(r'[_\-]+', s))


def go_field(name):
    parts = re.split(r'[_\-]+', name)
    return ''.join(_ABBREVS.get(p.lower(), p.capitalize()) for p in parts)


def ref_name(ref):
    return ref.rsplit('/', 1)[-1]


# ---------------------------------------------------------------------------
# Spec helpers
# ---------------------------------------------------------------------------

def is_paged(name, schemas):
    s = schemas.get(name, {})
    props = s.get('properties', {})
    return 'count' in props and 'results' in props and 'next' in props


def paged_item(name, schemas):
    s = schemas.get(name, {})
    items = s.get('properties', {}).get('results', {}).get('items', {})
    ref = items.get('$ref', '')
    return ref_name(ref) if ref else None


# ---------------------------------------------------------------------------
# Operation model
# ---------------------------------------------------------------------------

class Op:
    __slots__ = ('op_id', 'method', 'path', 'summary',
                 'path_params', 'query_params', 'header_params',
                 'body', 'resp_type', 'resp_schema', 'resp_code',
                 'is_list', 'item_type', 'no_body')

    def __init__(self):
        self.op_id = ''
        self.method = ''
        self.path = ''
        self.summary = ''
        self.path_params = []
        self.query_params = []
        self.header_params = []
        self.body = None
        self.resp_type = ''       # gen response wrapper, e.g. CreateVolumeResponse
        self.resp_schema = None   # payload type, e.g. VolumeDetailSchema
        self.resp_code = '200'
        self.is_list = False
        self.item_type = None
        self.no_body = False


def parse_ops(spec, tag):
    schemas = spec.get('components', {}).get('schemas', {})
    out = []
    for path, methods in spec.get('paths', {}).items():
        for method, op in methods.items():
            if method == 'parameters' or not isinstance(op, dict):
                continue
            if tag not in op.get('tags', []):
                continue

            o = Op()
            o.op_id = op.get('operationId', '')
            o.method = method.upper()
            o.path = path
            o.summary = op.get('summary', '')

            for p in op.get('parameters', []):
                name = p.get('name', '')
                where = p.get('in', '')
                fmt = p.get('schema', {}).get('format', '')
                info = {'go_name': go_field(name), 'param_name': name, 'format': fmt}
                if where == 'path':
                    o.path_params.append(info)
                elif where == 'query':
                    o.query_params.append(info)
                elif where == 'header':
                    o.header_params.append(info)

            rb = op.get('requestBody', {})
            for ct, info in rb.get('content', {}).items():
                if 'json' in ct:
                    ref = info.get('schema', {}).get('$ref', '')
                    if ref:
                        o.body = ref_name(ref)
                    break

            for code in ('200', '201', '202', '204'):
                resp = op.get('responses', {}).get(code, {})
                content = resp.get('content', {})
                for ct, info in content.items():
                    if 'json' in ct:
                        ref = info.get('schema', {}).get('$ref', '')
                        if ref:
                            sn = ref_name(ref)
                            o.resp_schema = sn
                            o.resp_code = code
                            o.resp_type = pascal(o.op_id) + 'Response'
                            if is_paged(sn, schemas):
                                o.is_list = True
                                o.item_type = paged_item(sn, schemas)
                        break
                if o.resp_schema:
                    break
                if not content and resp:
                    o.resp_code = code
                    o.no_body = True
                    break

            out.append(o)
    return out


# ---------------------------------------------------------------------------
# Code generators
# ---------------------------------------------------------------------------

# Prepended to every fully generated facade file so tooling and humans treat it
# as read-only. The command that (re)writes it is the `//go:generate` directive
# in doc.go, so `go generate ./...` regenerates. doc.go and README.md are
# hand-maintained and omit this marker.
_GEN_MARKER = '// Code generated by "go generate"; DO NOT EDIT.'


# ---- Per-service generators ----

_CLIENT_MARKER = '// RequestEditorFn is the function signature'


def discriminator_type_names(spec):
    """Type names oapi-codegen emits for `const` discriminator properties.

    A union member tags itself with a const field (source_type, kind, ...).
    oapi-codegen turns each such field into a single-value enum type named
    <Schema><Property> plus one constant. The union's From*/Merge* helpers write
    the tag to the wire automatically, so a caller never names these: choosing
    From<Member> already picks the variant, and the lone value carries no
    choice. The facade drops them so it exposes only types callers actually use.
    """
    schemas = spec.get('components', {}).get('schemas', {})
    names = set()
    for schema_name, schema in schemas.items():
        if not isinstance(schema, dict):
            continue
        for prop_name, prop in (schema.get('properties') or {}).items():
            if isinstance(prop, dict) and 'const' in prop:
                names.add(schema_name + pascal(prop_name))
    return names


def generated_model_symbols(generated_path):
    """Return the model types/constants emitted before client plumbing.

    oapi-codegen writes model declarations, enum constants, union wrappers and
    operation Params types before RequestEditorFn. Reading that generated Go
    surface gives the facade the exact transitive dependency closure, including
    inline union wrapper names that do not exist as component schemas in the
    OpenAPI document.
    """
    source = generated_path.read_text()
    marker_at = source.find(_CLIENT_MARKER)
    if marker_at < 0:
        raise ValueError(
            'could not find oapi-codegen client marker in %s' % generated_path
        )

    model_source = source[:marker_at]
    type_names = {
        name for name in re.findall(
            r'^type\s+([A-Z][A-Za-z0-9_]*)\b', model_source, re.MULTILINE
        )
        if not name.endswith('JSONRequestBody')
    }

    const_names = set()
    for block in re.findall(
            r'^const\s*\((.*?)^\)', model_source, re.MULTILINE | re.DOTALL):
        for line in block.splitlines():
            match = re.match(r'^\s*([A-Z][A-Za-z0-9_]*)\b.*=', line)
            if match:
                const_names.add(match.group(1))

    return sorted(type_names), sorted(const_names)


def gen_types(svc, type_names, const_names, module_path):
    lines = [_GEN_MARKER, '',
             'package ' + svc, '',
             'import gen "' + module_path + '/' + svc + '/internal/gen"', '']
    for name in type_names:
        lines.append('type ' + name + ' = gen.' + name)
    if const_names:
        lines.extend(['', 'const ('])
        for name in const_names:
            lines.append('\t' + name + ' = gen.' + name)
        lines.append(')')
    return '\n'.join(lines) + '\n'


def _indent(text, tab='\t'):
    return '\n'.join(tab + line if line.strip() else '' for line in text.split('\n'))


def gen_client(svc, ops, module_path):
    """Generate the public Client struct with functional options."""
    return _GEN_MARKER + '''

package ''' + svc + '''

import (
\t"context"
\t"log/slog"
\t"net/http"

\t"''' + module_path + '''/core"
\tgen "''' + module_path + '''/''' + svc + '''/internal/gen"
)

// ---------------------------------------------------------------------------
// Client
// ---------------------------------------------------------------------------

// Client wraps the generated API client and exposes typed facade methods.
type Client struct {
\tgen *gen.ClientWithResponses
}

// NewClient creates a new Client for the given server URL.
//
//	c, err := ''' + svc + '''.NewClient("<api-endpoint>",
//		''' + svc + '''.WithPAT(token),
//		''' + svc + '''.WithHTTPClient(customHTTPClient),
//		''' + svc + '''.WithRetry(core.DefaultRetry()),
//		''' + svc + '''.WithUserAgent("myapp/1.2.3"),
//		''' + svc + '''.WithLogger(slog.Default()),
//	)
func NewClient(server string, opts ...Option) (*Client, error) {
\tcfg := core.ResolveClientOptions(opts...)
\tvar genOpts []gen.ClientOption
\tfor _, editor := range cfg.RequestEditors {
\t\teditor := editor
\t\tgenOpts = append(genOpts, gen.WithRequestEditorFn(func(ctx context.Context, req *http.Request) error {
\t\t\treturn editor(ctx, req)
\t\t}))
\t}
\tgenOpts = append(genOpts, gen.WithHTTPClient(cfg.HTTPClient))

\tc, err := gen.NewClientWithResponses(server, genOpts...)
\tif err != nil {
\t\treturn nil, err
\t}
\treturn &Client{gen: c}, nil
}

// ---------------------------------------------------------------------------
// Options
// ---------------------------------------------------------------------------

// Option configures the service-independent portions of Client.
type Option = core.ClientOption

// WithPAT sets the Personal Access Token for authentication.
// It sets the Authorization: Token <token> header on every request.
func WithPAT(token string) Option {
\treturn core.WithPAT(token)
}

// WithHTTPClient sets a custom http.Client (for connection pooling, proxies, TLS, etc.).
func WithHTTPClient(c *http.Client) Option {
\treturn core.WithHTTPClient(c)
}

// WithRetry enables retries with the given config. Use core.DefaultRetry()
// for sensible defaults or core.NoRetry() to disable.
func WithRetry(cfg *core.RetryConfig) Option {
\treturn core.WithRetry(cfg)
}

// WithRetryAllMethods enables retries for all HTTP methods including POST.
// By default only idempotent methods (GET, PUT, DELETE) are retried.
func WithRetryAllMethods(retry bool) Option {
\treturn core.WithRetryAllMethods(retry)
}

// WithUserAgent sets the User-Agent header on every request.
func WithUserAgent(ua string) Option {
\treturn core.WithUserAgent(ua)
}

// WithLogger enables debug logging of method, path, status, duration and request ID.
func WithLogger(l *slog.Logger) Option {
\treturn core.WithLogger(l)
}

// WithRequestEditor adds a custom request editor that runs on every request.
func WithRequestEditor(fn core.RequestEditorFn) Option {
\treturn core.WithRequestEditor(fn)
}

// WithIdempotencyKey returns a context that adds Idempotency-Key to POST
// operations. Use it only where the API documents idempotency-key support.
func WithIdempotencyKey(ctx context.Context, key string) context.Context {
\treturn core.WithIdempotencyKey(ctx, key)
}
'''


def gen_service_error(svc, module_path):
    """Generate per-service error.go re-exporting sentinels from core."""
    return _GEN_MARKER + '''

package ''' + svc + '''

import "''' + module_path + '''/core"

// Sentinel errors re-exported from core for convenience.
// Use with errors.Is(err, ''' + svc + '''.ErrNotFound) etc.
var (
\tErrNotFound     = core.ErrNotFound
\tErrUnauthorized = core.ErrUnauthorized
\tErrConflict     = core.ErrConflict
\tErrValidation   = core.ErrValidation
\tErrRateLimited  = core.ErrRateLimited
\tErrInternal     = core.ErrInternal
)

// APIError is an alias for core.APIError — the structured error type
// returned by all facade methods on HTTP 4xx/5xx responses.
type APIError = core.APIError

// ErrorDetail is an alias for core.ErrorDetail — a single field-level error.
type ErrorDetail = core.ErrorDetail

// ResponseError reports an unexpected status or malformed success payload.
type ResponseError = core.ResponseError
'''


def gen_operations(svc, ops, module_path):
    list_ops = [o for o in ops if o.is_list and o.item_type]
    needs_iter = bool(list_ops)

    imports = ['\t"context"']
    if needs_iter:
        imports.append('\t"iter"')
    imports.extend([
        '',
        '\t"' + module_path + '/core"',
    ])

    lines = [_GEN_MARKER, '',
             'package ' + svc, '',
             'import (',
             '\n'.join(imports),
             ')', '']

    for o in ops:
        name = pascal(o.op_id)

        # --- Signature ---
        sig = ['ctx context.Context']
        for pp in o.path_params:
            if pp.get('format') == 'uuid':
                sig.append(pp['go_name'] + ' core.UUID')
            else:
                sig.append(pp['go_name'] + ' string')
        if o.query_params or o.header_params:
            sig.append('params ' + name + 'Params')
        if o.body:
            sig.append('body ' + o.body)

        # --- Return type ---
        if o.no_body:
            ret = 'error'
        elif o.resp_schema:
            ret = '*' + o.resp_schema + ', error'
        else:
            ret = 'error'

        summary = o.summary or (o.method + ' ' + o.path)
        lines.append('// ' + name + ' ' + summary)
        lines.append('//')
        lines.append('// ' + o.method + ' ' + o.path)
        if ret.count(',') == 0:
            # Single return value — no parens
            lines.append('func (c *Client) ' + name + '(' + ', '.join(sig) + ') ' + ret + ' {')
        else:
            lines.append('func (c *Client) ' + name + '(' + ', '.join(sig) + ') (' + ret + ') {')

        # --- Build gen params ---
        # Since facade types are aliases of gen types, pass directly.
        has_params = bool(o.query_params or o.header_params)
        if has_params:
            lines.append('\tgenParams := params')

        # --- Call the generated WithResponse method ---
        call = ['ctx']
        for pp in o.path_params:
            call.append(pp['go_name'])
        if has_params:
            call.append('&genParams')
        if o.body:
            call.append('body')
        call_str = ', '.join(call)

        lines.append('\tresp, err := c.gen.' + name + 'WithResponse(' + call_str + ')')
        lines.append('\tif err != nil {')
        if o.no_body:
            lines.append('\t\treturn err')
        else:
            lines.append('\t\treturn nil, err')
        lines.append('\t}')

        # --- Parse response ---
        json_field = 'JSON' + o.resp_code
        if o.no_body:
            lines.append('\treturn core.ParseEmptyResponse(resp.HTTPResponse, resp.Body, ' + o.resp_code + ')')
        elif o.resp_schema:
            lines.append('\treturn core.ParseResponse(resp.HTTPResponse, resp.Body, resp.' + json_field + ', ' + o.resp_code + ')')

        lines.append('}')
        lines.append('')

    # --- List*Iter methods for paginated operations ---
    for o in list_ops:
        name = pascal(o.op_id)
        item = o.item_type

        # Signature: same as the List* method
        iter_sig = ['ctx context.Context']
        for pp in o.path_params:
            if pp.get('format') == 'uuid':
                iter_sig.append(pp['go_name'] + ' core.UUID')
            else:
                iter_sig.append(pp['go_name'] + ' string')
        if o.query_params or o.header_params:
            iter_sig.append('params ' + name + 'Params')

        # Build call args for the inner List* call
        inner_call = ['ctx']
        for pp in o.path_params:
            inner_call.append(pp['go_name'])
        if o.query_params or o.header_params:
            inner_call.append('p')

        lines.append('// ' + name + 'Iter returns an iterator over all ' + item + ' pages.')
        lines.append('// Automatically advances pages until all results are exhausted.')
        lines.append('func (c *Client) ' + name + 'Iter(' + ', '.join(iter_sig) + ') iter.Seq2[*' + item + ', error] {')
        lines.append('\treturn core.Paginate(ctx, func(ctx context.Context, page, size int) (core.Page[' + item + '], error) {')
        lines.append('\t\tp := params')
        lines.append('\t\tp.PageNumber = &page')
        lines.append('\t\tp.PageSize = &size')
        lines.append('\t\tpageResult, err := c.' + name + '(' + ', '.join(inner_call) + ')')
        lines.append('\t\tif err != nil {')
        lines.append('\t\t\treturn core.Page[' + item + ']{}, err')
        lines.append('\t\t}')
        lines.append('\t\treturn core.Page[' + item + ']{Count: pageResult.Count, Results: pageResult.Results}, nil')
        lines.append('\t})')
        lines.append('}')
        lines.append('')

    return '\n'.join(lines) + '\n'


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(description='Generate SDK facade from OpenAPI spec')
    parser.add_argument('spec_json', help='Path to openapi_edited.json')
    parser.add_argument('target', help='Service directory')
    parser.add_argument('tag',
                        help="OpenAPI tag selecting this service's operations "
                             '(main.py passes the `tag` from pipeline.yaml)')
    parser.add_argument(
        '--module-path',
        required=True,
        help='Canonical root Go module path',
    )
    args = parser.parse_args()

    spec_path = args.spec_json
    with open(spec_path) as f:
        spec = json.load(f)

    target = Path(args.target)
    svc = target.name
    if svc == 'core':
        parser.error('core is maintained manually and is not a facade target')

    tag = args.tag

    ops = parse_ops(spec, tag)
    print('  [%s] tag="%s" -> %d operations' % (svc, tag, len(ops)))

    target.mkdir(parents=True, exist_ok=True)

    generated_path = target / 'internal' / 'gen' / (svc + '.gen.go')
    type_names, const_names = generated_model_symbols(generated_path)

    # Drop const discriminator enums: the union From*/Merge* helpers set the tag
    # on the wire, so callers never name the type or its single constant.
    discriminators = discriminator_type_names(spec)
    hidden = [t for t in type_names if t in discriminators]
    type_names = [t for t in type_names if t not in discriminators]
    const_names = [c for c in const_names
                   if not any(c == d or c.startswith(d) for d in discriminators)]

    (target / 'client.go').write_text(
        gen_client(svc, ops, args.module_path)
    )
    print('  [%s] wrote client.go' % (svc,))

    (target / 'types.go').write_text(
        gen_types(svc, type_names, const_names, args.module_path)
    )
    print('  [%s] wrote types.go (%d type aliases, %d constants%s)' % (
        svc, len(type_names), len(const_names),
        ', %d discriminators hidden' % len(hidden) if hidden else ''))

    (target / 'operations.go').write_text(
        gen_operations(svc, ops, args.module_path)
    )
    print('  [%s] wrote operations.go (%d methods)' % (svc, len(ops)))

    (target / 'error.go').write_text(
        gen_service_error(svc, args.module_path)
    )
    print('  [%s] wrote error.go' % (svc,))

    # doc.go and README.md are hand-maintained; the facade never touches them.


if __name__ == '__main__':
    main()
