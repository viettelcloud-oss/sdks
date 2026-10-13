import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

import gen_facade


def json_response(ref='#/components/schemas/Payload'):
    return {
        'description': 'Success',
        'content': {'application/json': {'schema': {'$ref': ref}}},
    }


def spec_with_responses(responses):
    return {
        'info': {'version': '1.0.0'},
        'paths': {
            '/example': {
                'get': {
                    'operationId': 'get_example',
                    'tags': ['Example'],
                    'responses': responses,
                },
            },
        },
        'components': {'schemas': {'Payload': {'type': 'object'}}},
    }


class ParseSuccessResponsesTest(unittest.TestCase):
    def parse(self, responses):
        return gen_facade.parse_ops(spec_with_responses(responses), 'Example')[0]

    def test_collects_all_documented_success_codes_in_numeric_order(self):
        responses = {
            code: json_response()
            for code in ('299', '207', '206', '203', '202', '201', '200')
        }
        responses.update({'400': {'description': 'Error'},
                          'default': {'description': 'Unexpected response'}})

        op = self.parse(responses)

        self.assertEqual(op.resp_codes, ['200', '201', '202', '203', '206', '207', '299'])
        self.assertEqual(op.resp_schema, 'Payload')
        self.assertFalse(op.no_body)

    def test_accepts_an_uncommon_status_as_the_only_success_response(self):
        for code in ('203', '206', '207'):
            with self.subTest(code=code):
                op = self.parse({code: json_response()})
                self.assertEqual(op.resp_codes, [code])
                self.assertEqual(op.resp_schema, 'Payload')
                self.assertFalse(op.no_body)

    def test_collects_documented_empty_responses(self):
        op = self.parse({code: {'description': 'Success'}
                         for code in ('205', '204', '202')})

        self.assertEqual(op.resp_codes, ['202', '204', '205'])
        self.assertIsNone(op.resp_schema)
        self.assertTrue(op.no_body)

    def test_missing_success_response_does_not_default_to_200(self):
        for responses in ({}, {'400': {'description': 'Error'}},
                          {'default': json_response()}):
            with self.subTest(responses=responses):
                with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                            'no explicit success status'):
                    self.parse(responses)

    def test_rejects_success_range_even_with_an_explicit_code(self):
        for responses in ({'2XX': json_response()},
                          {'200': json_response(), '2XX': json_response()}):
            with self.subTest(responses=responses):
                with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                            'response range 2XX is not supported'):
                    self.parse(responses)

    def test_rejects_mixed_json_and_empty_responses(self):
        cases = (
            {'200': json_response(), '204': {'description': 'No content'}},
            {'200': {'description': 'No content'}, '202': json_response()},
        )
        for responses in cases:
            with self.subTest(responses=responses):
                with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                            'mix JSON bodies .* and empty bodies'):
                    self.parse(responses)

    def test_rejects_different_schema_references(self):
        for ref in ('#/components/schemas/Other', 'other.json#/components/schemas/Payload'):
            with self.subTest(ref=ref):
                with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                            '200 and 202 use different JSON schema references'):
                    self.parse({'200': json_response(), '202': json_response(ref)})

    def test_does_not_discard_unsupported_bodies(self):
        contents = (
            {'application/json': {'schema': {'type': 'object'}}},
            {'text/plain': {'schema': {'type': 'string'}}},
        )
        for content in contents:
            for include_json200 in (False, True):
                with self.subTest(content=content, include_json200=include_json200):
                    responses = {'207': {'description': 'Success', 'content': content}}
                    if include_json200:
                        responses['200'] = json_response()
                    with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                                'success response 207 has no supported JSON'):
                        self.parse(responses)

    def test_response_reference_does_not_become_an_empty_response(self):
        with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                    'success response 200 uses a response reference'):
            self.parse({'200': {'$ref': '#/components/responses/Success'}})

    def test_ignores_operations_outside_the_selected_tag(self):
        spec = spec_with_responses({'2XX': json_response()})
        self.assertEqual(gen_facade.parse_ops(spec, 'Other'), [])

    def test_preserves_pagination_for_uncommon_success_status(self):
        spec = spec_with_responses({'203': json_response()})
        spec['components']['schemas']['Payload'] = {
            'type': 'object',
            'properties': {
                'count': {'type': 'integer'},
                'next': {'type': 'string'},
                'results': {'type': 'array',
                            'items': {'$ref': '#/components/schemas/Item'}},
            },
        }

        op = gen_facade.parse_ops(spec, 'Example')[0]

        self.assertTrue(op.is_list)
        self.assertEqual(op.item_type, 'Item')
        self.assertEqual(op.resp_codes, ['203'])

    def test_cli_failure_preserves_existing_facade_files(self):
        spec = spec_with_responses({'200': json_response()})
        spec['paths']['/unsupported'] = {
            'get': {'operationId': 'get_unsupported', 'tags': ['Example'],
                    'responses': {'2XX': json_response()}},
        }
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            spec_path = root / 'spec.json'
            spec_path.write_text(json.dumps(spec))
            target = root / 'example'
            target.mkdir()
            for name in ('client.go', 'types.go', 'operations.go', 'error.go'):
                (target / name).write_text('existing ' + name)

            result = subprocess.run(
                [sys.executable, gen_facade.__file__, str(spec_path), str(target),
                 'Example', '--module-path', 'example.com/sdk'],
                capture_output=True, text=True,
            )

            self.assertEqual(result.returncode, 1)
            self.assertIn('ERROR: GET /unsupported (get_unsupported):', result.stderr)
            self.assertIn('response range 2XX is not supported', result.stderr)
            self.assertNotIn('Traceback', result.stderr)
            for name in ('client.go', 'types.go', 'operations.go', 'error.go'):
                self.assertEqual((target / name).read_text(), 'existing ' + name)


if __name__ == '__main__':
    unittest.main()
