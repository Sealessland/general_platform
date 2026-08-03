"""校验 prompts 目录下必需的提示词模板是否齐全，供 CI/本地脚本调用。

prompts 目录基于本文件位置解析（ai-service/prompts），与运行时的当前工作目录无关。
"""

from pathlib import Path


# 服务端各功能依赖的提示词模板清单，与 prompts/ 目录一一对应。
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
    """检查模板文件是否存在，缺失时以非零退出码报错。"""
    prompt_dir = Path(__file__).resolve().parent.parent / "prompts"
    missing = [name for name in REQUIRED_PROMPTS if not (prompt_dir / name).is_file()]
    if missing:
        raise SystemExit(f"missing prompt templates: {', '.join(missing)}")
    print("prompt templates passed")


if __name__ == "__main__":
    main()
