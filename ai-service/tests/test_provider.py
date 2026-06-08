import unittest

from app.provider import BusinessReviewRequest, MockAIProvider, SellingPointRequest


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

    def test_generate_business_review(self) -> None:
        provider = MockAIProvider()
        review = provider.generate_business_review(
            BusinessReviewRequest(window_days=7, gmv=12900, refund_rate=0.25)
        )
        self.assertEqual(review["window_days"], 7)
        self.assertEqual(review["gmv"], 12900)
        self.assertIn("diagnosis", review)
        self.assertGreater(len(review["next_steps"]), 0)

    def test_reject_invalid_business_review_window(self) -> None:
        provider = MockAIProvider()
        with self.assertRaises(ValueError):
            provider.generate_business_review(BusinessReviewRequest(window_days=0))


if __name__ == "__main__":
    unittest.main()
