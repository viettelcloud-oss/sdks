from contextlib import redirect_stderr
import io
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


def page_number_pagination(page='page_number', page_size='page_size',
                           items='results', total='count'):
    return {
        'type': 'page_number',
        'request': {'page': page, 'page_size': page_size},
        'response': {'items': items, 'total': total},
    }


def paged_spec(pagination, query=('page_number', 'page_size'), code='200'):
    """A list operation over Item, with the given x-pagination and query params."""
    operation = {
        'operationId': 'list_items',
        'tags': ['Example'],
        'parameters': [{'name': name, 'in': 'query', 'schema': {'type': 'integer'}}
                       for name in query],
        'responses': {code: json_response()},
    }
    if pagination is not None:
        operation['x-pagination'] = pagination
    return {
        'info': {'version': '1.0.0'},
        'paths': {'/items': {'get': operation}},
        'components': {
            'schemas': {
                'Payload': {
                    'type': 'object',
                    'properties': {
                        'count': {'type': 'integer'},
                        'results': {'type': 'array',
                                    'items': {'$ref': '#/components/schemas/Item'}},
                    },
                },
                'Item': {'type': 'object'},
            },
        },
    }


class PaginationTest(unittest.TestCase):
    def generate(self, spec):
        ops = gen_facade.parse_ops(spec, 'Example')
        return gen_facade.gen_operations('example', ops, 'example.com/sdk')

    def test_iterator_sets_the_parameters_x_pagination_names(self):
        for page, field in (('page_number', 'PageNumber'), ('page', 'Page')):
            with self.subTest(page=page):
                spec = paged_spec(page_number_pagination(page=page),
                                  query=(page, 'page_size'))
                code = self.generate(spec)
                self.assertIn(f'p.{field} = &page', code)
                self.assertIn('p.PageSize = &size', code)
                self.assertIn('Count: pageResult.Count, Results: pageResult.Results', code)

    def test_a_size_filter_is_never_taken_for_the_page_size(self):
        """`size` is the volume size filter on block storage lists."""
        spec = paged_spec(page_number_pagination(),
                          query=('size', 'page', 'page_number', 'page_size'))
        code = self.generate(spec)
        self.assertIn('p.PageNumber = &page', code)
        self.assertIn('p.PageSize = &size', code)
        self.assertNotIn('p.Size =', code)
        self.assertNotIn('p.Page =', code)

    def test_pagination_survives_an_uncommon_success_status(self):
        op = gen_facade.parse_ops(paged_spec(page_number_pagination(), code='203'),
                                  'Example')[0]
        self.assertTrue(op.is_list)
        self.assertEqual(op.item_type, 'Item')
        self.assertEqual(op.resp_codes, ['203'])

    def test_a_list_answered_whole_gets_no_iterator(self):
        spec = paged_spec({'type': 'none', 'request': {},
                           'response': {'items': 'results', 'total': 'count'}}, query=())
        op = gen_facade.parse_ops(spec, 'Example')[0]
        self.assertFalse(op.is_list)
        self.assertNotIn('ListItemsIter', self.generate(spec))

    def test_an_unknown_type_gets_no_iterator_and_a_warning(self):
        spec = paged_spec({'type': 'offset_limit', 'request': {}, 'response': {}})
        with redirect_stderr(io.StringIO()) as stderr:
            op = gen_facade.parse_ops(spec, 'Example')[0]
        self.assertFalse(op.is_list)
        self.assertIn("unknown x-pagination type 'offset_limit'", stderr.getvalue())

    def test_a_list_page_without_x_pagination_is_an_error(self):
        with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                    'list page but the operation has no x-pagination'):
            gen_facade.parse_ops(paged_spec(None), 'Example')

    def test_a_name_that_is_not_a_query_parameter_is_an_error(self):
        spec = paged_spec(page_number_pagination(page='page'))
        with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                    "request page names 'page', which is not a query"):
            gen_facade.parse_ops(spec, 'Example')

    def test_a_name_that_is_not_a_response_field_is_an_error(self):
        spec = paged_spec(page_number_pagination(total='total'))
        with self.assertRaisesRegex(gen_facade.FacadeGenerationError,
                                    "response total names 'total', which is not a field"):
            gen_facade.parse_ops(spec, 'Example')


if __name__ == '__main__':
    unittest.main()
