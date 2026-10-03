import importlib.util
import pathlib
import unittest


path = pathlib.Path(__file__).with_name("check_publish.py")
spec = importlib.util.spec_from_file_location("check_publish", path)
publisher = importlib.util.module_from_spec(spec)
spec.loader.exec_module(publisher)


class PublishPolicyTests(unittest.TestCase):
    def test_revisions_are_numeric(self):
        self.assertGreater(publisher.parse_version("0.2.13-r10"), publisher.parse_version("0.2.13-r9"))

    def test_invalid_identity_is_rejected(self):
        for value in ("0.2.13", "0.2.13-r0", "0.2.13-r01", "../0.2.13-r1"):
            with self.assertRaises(ValueError):
                publisher.parse_version(value)

    def test_same_version_cannot_be_republished(self):
        self.assertFalse(publisher.should_publish("0.2.13-r2", "0.2.13-r2"))
        self.assertFalse(publisher.should_publish("0.2.13-r1", "0.2.13-r2"))
        self.assertTrue(publisher.should_publish("0.2.13-r3", "0.2.13-r2"))
        self.assertTrue(publisher.should_publish("0.2.13-r1", None))


if __name__ == "__main__":
    unittest.main()
