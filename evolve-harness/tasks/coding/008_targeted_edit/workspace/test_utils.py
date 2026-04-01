"""Tests for utils.py — exercises key functions to catch regressions."""

import sys
import os

# Ensure we can import from the same directory
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from utils import (
    slugify,
    truncate,
    camel_to_snake,
    snake_to_camel,
    mask_email,
    extract_numbers,
    flatten_dict,
    unflatten_dict,
    chunk_list,
    deep_merge,
    is_valid_email,
    clamp,
    group_by,
    unique_by,
    pluck,
)


def test_slugify():
    assert slugify("Hello World!") == "hello-world"
    assert slugify("  Foo  Bar  ") == "foo-bar"
    assert slugify("CamelCase Test") == "camelcase-test"
    print("  PASS: test_slugify")


def test_truncate():
    assert truncate("short", 100) == "short"
    assert truncate("hello world", 8) == "hello..."
    assert len(truncate("a" * 200, 50)) == 50
    print("  PASS: test_truncate")


def test_case_conversion():
    assert camel_to_snake("camelCase") == "camel_case"
    assert camel_to_snake("PascalCase") == "pascal_case"
    assert snake_to_camel("snake_case") == "snakeCase"
    assert snake_to_camel("snake_case", pascal=True) == "SnakeCase"
    print("  PASS: test_case_conversion")


def test_mask_email():
    assert mask_email("john@example.com") == "j***@example.com"
    assert mask_email("a@b.com") == "*@b.com"
    print("  PASS: test_mask_email")


def test_extract_numbers():
    nums = extract_numbers("I have 3 cats and 2.5 dogs")
    assert 3.0 in nums
    assert 2.5 in nums
    print("  PASS: test_extract_numbers")


def test_flatten_dict():
    """This test calls flatten_dict which has the typo bug."""
    nested = {"a": {"b": 1, "c": {"d": 2}}, "e": 3}
    flat = flatten_dict(nested)
    assert flat == {"a.b": 1, "a.c.d": 2, "e": 3}
    print("  PASS: test_flatten_dict")


def test_unflatten_dict():
    flat = {"a.b": 1, "a.c.d": 2, "e": 3}
    nested = unflatten_dict(flat)
    assert nested == {"a": {"b": 1, "c": {"d": 2}}, "e": 3}
    print("  PASS: test_unflatten_dict")


def test_chunk_list():
    chunks = chunk_list([1, 2, 3, 4, 5], 2)
    assert chunks == [[1, 2], [3, 4], [5]]
    print("  PASS: test_chunk_list")


def test_deep_merge():
    base = {"a": 1, "b": {"c": 2, "d": 3}}
    override = {"b": {"c": 99, "e": 4}, "f": 5}
    merged = deep_merge(base, override)
    assert merged == {"a": 1, "b": {"c": 99, "d": 3, "e": 4}, "f": 5}
    print("  PASS: test_deep_merge")


def test_is_valid_email():
    assert is_valid_email("user@example.com")
    assert not is_valid_email("not-an-email")
    assert not is_valid_email("@missing.com")
    print("  PASS: test_is_valid_email")


def test_clamp():
    assert clamp(5, 1, 10) == 5
    assert clamp(-5, 0, 100) == 0
    assert clamp(999, 0, 100) == 100
    print("  PASS: test_clamp")


def test_group_by():
    items = [
        {"name": "Alice", "dept": "eng"},
        {"name": "Bob", "dept": "eng"},
        {"name": "Carol", "dept": "sales"},
    ]
    groups = group_by(items, "dept")
    assert len(groups["eng"]) == 2
    assert len(groups["sales"]) == 1
    print("  PASS: test_group_by")


def test_unique_by():
    items = [
        {"id": 1, "name": "Alice"},
        {"id": 2, "name": "Bob"},
        {"id": 1, "name": "Alice (dup)"},
    ]
    result = unique_by(items, "id")
    assert len(result) == 2
    assert result[0]["name"] == "Alice"
    print("  PASS: test_unique_by")


def test_pluck():
    items = [{"a": 1}, {"a": 2}, {"b": 3}]
    assert pluck(items, "a") == [1, 2, None]
    print("  PASS: test_pluck")


if __name__ == "__main__":
    tests = [
        test_slugify,
        test_truncate,
        test_case_conversion,
        test_mask_email,
        test_extract_numbers,
        test_flatten_dict,     # This one will hit the bug
        test_unflatten_dict,
        test_chunk_list,
        test_deep_merge,
        test_is_valid_email,
        test_clamp,
        test_group_by,
        test_unique_by,
        test_pluck,
    ]

    print(f"Running {len(tests)} tests...\n")
    passed = 0
    failed = 0
    for test in tests:
        try:
            test()
            passed += 1
        except Exception as e:
            print(f"  FAIL: {test.__name__}: {e}")
            failed += 1

    print(f"\n{passed} passed, {failed} failed")
    if failed > 0:
        sys.exit(1)
    print("All tests passed!")
