import unittest

from app.provider import MockAIProvider, SellingPointRequest


class MockAIProviderTest(unittest.TestCase):
    def test_generate_selling_points(self) -> None:
        provider = MockAIProvider()
        points = provider.generate_selling_points(
            SellingPointRequest(product_name="Travel Makeup Bag", audience="dorm users")
        )
        self.assertGreater(len(points), 0)

    def test_reject_empty_product_name(self) -> None:
        provider = MockAIProvider()
        with self.assertRaises(ValueError):
            provider.generate_selling_points(
                SellingPointRequest(product_name="", audience="dorm users")
            )


if __name__ == "__main__":
    unittest.main()
