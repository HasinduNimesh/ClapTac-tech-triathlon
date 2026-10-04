import importlib.util
import io
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from pathlib import Path

MODULE_PATH = Path(__file__).with_name("import-outlet-locations.py")
SPEC = importlib.util.spec_from_file_location("waypoint_import_outlet_locations", MODULE_PATH)
importer = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(importer)


class ReadRowsTests(unittest.TestCase):
    def read(self, text):
        return importer.read_rows(io.StringIO(text))

    def test_valid_rows_with_extra_columns_and_blank_lines(self):
        rows, problems = self.read("outlet_id,name,latitude,longitude\nOUT001,Pettah,6.93441,79.84281\n,,,\nOUT002,Kandy,7.2906,80.6337\n")
        self.assertEqual(problems, [])
        self.assertEqual(rows, [("OUT001", 6.93441, 79.84281), ("OUT002", 7.2906, 80.6337)])

    def test_missing_columns_are_named(self):
        rows, problems = self.read("outlet_id,lat\nOUT001,6.9\n")
        self.assertEqual(rows, [])
        self.assertIn("latitude, longitude", problems[0])

    def test_bad_rows_are_each_reported_with_their_line(self):
        _, problems = self.read(
            "outlet_id,latitude,longitude\n"
            "OUT001,79.84,6.93\n"        # swapped
            "OUT002,51.5,-0.12\n"        # abroad
            "OUT003,six,79.8\n"          # not a number
            "OUT004,6.9,79.8\n"
            "OUT004,6.9,79.8\n"          # repeated
            ",6.9,79.8\n"                # no outlet
        )
        text = "\n".join(problems)
        self.assertIn("line 2 (OUT001): latitude and longitude look swapped", text)
        self.assertIn("line 3 (OUT002): 51.5, -0.12 is not in Sri Lanka", text)
        self.assertIn("line 4 (OUT003): latitude and longitude must be numbers", text)
        self.assertIn("line 6 (OUT004): repeats the outlet on line 5", text)
        self.assertIn("line 7 (no outlet_id): outlet_id is empty", text)
        self.assertEqual(len(problems), 5)


class UpdateBodyTests(unittest.TestCase):
    def test_body_keeps_the_outlet_and_trims_times_to_what_the_api_accepts(self):
        outlet = {"name": "Pettah", "brand": "Fresh", "district": "Colombo", "depot": "DEPOT_NORTH", "dockType": "normal",
                  "parkingConstraint": "van_only", "mallWindow": False, "windowOpenTime": "08:00:00", "windowCloseTime": "",
                  "accessInstructions": "East gate", "chilledTemperatureMinC": None, "chilledTemperatureMaxC": None, "version": 4,
                  "latitude": 6.9, "longitude": 79.9, "locationApproximate": True}
        body = importer.update_body(outlet, 6.93441, 79.84281)
        self.assertEqual(body["windowOpenTime"], "08:00")
        self.assertEqual(body["windowCloseTime"], "")
        self.assertEqual(body["version"], 4)
        self.assertEqual(body["location"], {"latitude": 6.93441, "longitude": 79.84281})
        self.assertNotIn("latitude", body, "the approximate position read must not be echoed back")


class FakeApi(BaseHTTPRequestHandler):
    outlets = {}
    puts = []

    def log_message(self, *args):
        pass

    def _send(self, status, payload):
        data = json.dumps(payload).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        outlet = self.outlets.get(self.path.rsplit("/", 1)[-1])
        if self.headers.get("Authorization") != "Bearer secret-token":
            return self._send(401, {})
        self._send(200 if outlet else 404, {"outlet": outlet} if outlet else {})

    def do_PUT(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        outlet_id = self.path.rsplit("/", 1)[-1]
        self.puts.append((outlet_id, body))
        self._send(409 if outlet_id == "OUT003" else 200, {"outlet": {}})


class RunTests(unittest.TestCase):
    def setUp(self):
        FakeApi.puts = []
        FakeApi.outlets = {
            "OUT001": {"version": 1, "name": "A", "locationApproximate": True, "latitude": 6.9, "longitude": 79.9},
            "OUT002": {"version": 2, "name": "B", "locationApproximate": False, "latitude": 7.2906, "longitude": 80.6337},
            "OUT003": {"version": 3, "name": "C", "locationApproximate": True, "latitude": 6.9, "longitude": 79.9},
        }
        self.server = HTTPServer(("127.0.0.1", 0), FakeApi)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.api = f"http://127.0.0.1:{self.server.server_port}/api/v1"
        self.addCleanup(self.server.shutdown)
        self.rows = [("OUT001", 6.93441, 79.84281), ("OUT002", 7.2906, 80.6337), ("OUT003", 6.95, 79.86), ("OUT404", 6.9, 79.8)]
        self.lines = []

    def test_check_only_sends_nothing(self):
        result = importer.run(self.rows, self.api, "secret-token", apply=False, out=self.lines.append)
        self.assertEqual(result, (2, 1, 1))
        self.assertEqual(FakeApi.puts, [])
        self.assertTrue(any("[would set] OUT001" in line for line in self.lines))
        self.assertTrue(any("[same] OUT002" in line for line in self.lines))

    def test_apply_saves_changed_outlets_and_reports_conflicts_and_missing_ones(self):
        result = importer.run(self.rows, self.api, "secret-token", apply=True, out=self.lines.append)
        self.assertEqual(result, (1, 1, 2))
        self.assertEqual([outlet for outlet, _ in FakeApi.puts], ["OUT001", "OUT003"])
        self.assertEqual(FakeApi.puts[0][1]["location"], {"latitude": 6.93441, "longitude": 79.84281})
        text = "\n".join(self.lines)
        self.assertIn("OUT003: changed by someone else", text)
        self.assertIn("OUT404: could not read the outlet (HTTP 404)", text)

    def test_the_token_is_never_printed(self):
        importer.run(self.rows, self.api, "secret-token", apply=True, out=self.lines.append)
        self.assertNotIn("secret-token", "\n".join(self.lines))


if __name__ == "__main__":
    unittest.main()
