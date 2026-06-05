from pathlib import Path


REQUIRED_PROMPTS = [
    "requirement_breakdown.md",
    "api_design.md",
    "test_case_generation.md",
    "code_review.md",
    "sql_review.md",
    "refactor_plan.md",
    "incident_analysis.md",
    "business_review.md",
]


def main() -> None:
    prompt_dir = Path("prompts")
    missing = [name for name in REQUIRED_PROMPTS if not (prompt_dir / name).is_file()]
    if missing:
        raise SystemExit(f"missing prompt templates: {', '.join(missing)}")
    print("prompt templates passed")


if __name__ == "__main__":
    main()
