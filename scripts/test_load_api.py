import importlib.util
import unittest
from pathlib import Path


MODULE_PATH = Path(__file__).with_name("load-api.py")
SPEC = importlib.util.spec_from_file_location("waypoint_load_api", MODULE_PATH)
load_api = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(load_api)


class StatusCheckTests(unittest.TestCase):
    def test_expected_statuses_have_no_errors(self):
        errors, missing = load_api.check_statuses([200, 200], 200)
        self.assertEqual(errors, 0)
        self.assertEqual(missing, set())

    def test_rate_limited_responses_can_be_accepted_and_required(self):
        errors, missing = load_api.check_statuses([200, 503, 200], 200, [503], [503])
        self.assertEqual(errors, 0)
        self.assertEqual(missing, set())

    def test_required_status_must_appear(self):
        errors, missing = load_api.check_statuses([200, 200], 200, [503], [503])
        self.assertEqual(errors, 0)
        self.assertEqual(missing, {503})

    def test_unapproved_status_is_an_error(self):
        errors, missing = load_api.check_statuses([200, 429], 200, [503])
        self.assertEqual(errors, 1)
        self.assertEqual(missing, set())


if __name__ == "__main__":
    unittest.main()
