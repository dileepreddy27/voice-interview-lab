import json
import threading
import unittest
from http.server import ThreadingHTTPServer
from unittest.mock import patch
from urllib.error import HTTPError
from urllib.request import Request, urlopen

from coach.service import Handler, evaluate, record_metrics


class EvaluationTests(unittest.TestCase):
    def test_signals_and_metrics(self):
        result = evaluate('Our team needed a fix. I built a cache and reduced calls by 20%. Um.', 30)
        self.assertTrue(all(result['structure_signals'].values()))
        self.assertTrue(result['quantified_detail'])
        self.assertEqual(result['filler_count'], 1)
        self.assertEqual(result['words_per_minute'], result['word_count'] * 2)

    def test_missing_structure_is_not_invented(self):
        result = evaluate('hello there', 2)
        self.assertFalse(any(result['structure_signals'].values()))
        self.assertIn('situation', result['tips'][0])

    def test_invalid_inputs(self):
        for duration in (0, -1, 121, float('nan'), float('inf'), True, '3'):
            with self.subTest(duration=duration), self.assertRaises(ValueError):
                evaluate('hello', duration)
        for text in (None, 'x' * 24001):
            with self.assertRaises(ValueError):
                evaluate(text, 1)

    def test_empty_transcript(self):
        result = evaluate('', 1)
        self.assertEqual(result['word_count'], 0)
        self.assertIn('No finalized speech', result['tips'][0])

    @patch.dict('os.environ', {}, clear=True)
    def test_storage_off_by_default(self):
        self.assertEqual(record_metrics(evaluate('hello', 1)), 'disabled')

    @patch.dict('os.environ', {'REDIS_URL': 'redis://127.0.0.1:1'})
    def test_storage_failure_does_not_fail_coaching(self):
        self.assertEqual(record_metrics(evaluate('hello', 1)), 'unavailable')


class HTTPTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        cls.thread = threading.Thread(target=cls.server.serve_forever, daemon=True)
        cls.thread.start()
        cls.url = f'http://127.0.0.1:{cls.server.server_port}'

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()
        cls.server.server_close()
        cls.thread.join()

    def test_http_contract(self):
        request = Request(self.url + '/evaluate', data=json.dumps({'text': 'hello', 'duration_seconds': 1}).encode())
        with urlopen(request) as response:
            self.assertEqual(json.load(response)['word_count'], 1)

    def test_bad_json_rejected(self):
        with self.assertRaises(HTTPError) as caught:
            urlopen(Request(self.url + '/evaluate', data=b'{bad'))
        self.assertEqual(caught.exception.code, 400)


if __name__ == '__main__':
    unittest.main()
