"""
utils.py - General-purpose utility functions.

A collection of commonly used helpers for string manipulation,
file operations, data transformation, and validation.
"""

import os
import re
import hashlib
import json
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple, Union


# =============================================================================
# String Utilities
# =============================================================================

def slugify(text: str) -> str:
    """Convert text to URL-friendly slug.

    Args:
        text: The input string to slugify.

    Returns:
        A lowercase, hyphenated slug string.
    """
    text = text.lower().strip()
    text = re.sub(r'[^\w\s-]', '', text)
    text = re.sub(r'[-\s]+', '-', text)
    return text.strip('-')


def truncate(text: str, max_length: int = 100, suffix: str = "...") -> str:
    """Truncate text to max_length, adding suffix if truncated.

    Args:
        text: The string to truncate.
        max_length: Maximum length of the result including suffix.
        suffix: String to append when truncated.

    Returns:
        The truncated (or original) string.
    """
    if len(text) <= max_length:
        return text
    return text[:max_length - len(suffix)] + suffix


def camel_to_snake(name: str) -> str:
    """Convert camelCase or PascalCase to snake_case.

    Args:
        name: A camelCase or PascalCase string.

    Returns:
        The snake_case equivalent.
    """
    s1 = re.sub(r'(.)([A-Z][a-z]+)', r'\1_\2', name)
    return re.sub(r'([a-z0-9])([A-Z])', r'\1_\2', s1).lower()


def snake_to_camel(name: str, pascal: bool = False) -> str:
    """Convert snake_case to camelCase or PascalCase.

    Args:
        name: A snake_case string.
        pascal: If True, produce PascalCase instead of camelCase.

    Returns:
        The camelCase (or PascalCase) equivalent.
    """
    components = name.split('_')
    if pascal:
        return ''.join(x.title() for x in components)
    return components[0] + ''.join(x.title() for x in components[1:])


def mask_email(email: str) -> str:
    """Mask an email address for display (e.g., j***@example.com).

    Args:
        email: The email address to mask.

    Returns:
        The masked email string.
    """
    if '@' not in email:
        return email
    local, domain = email.split('@', 1)
    if len(local) <= 1:
        return f"*@{domain}"
    return f"{local[0]}{'*' * (len(local) - 1)}@{domain}"


def extract_numbers(text: str) -> List[float]:
    """Extract all numbers (int and float) from a string.

    Args:
        text: The input string.

    Returns:
        A list of numbers found in the text.
    """
    pattern = r'-?\d+\.?\d*'
    return [float(x) for x in re.findall(pattern, text)]


def word_count(text: str) -> Dict[str, int]:
    """Count occurrences of each word in the text.

    Args:
        text: The input string.

    Returns:
        A dictionary mapping words to their counts.
    """
    words = re.findall(r'\b\w+\b', text.lower())
    counts = {}
    for w in words:
        counts[w] = counts.get(w, 0) + 1
    return counts


# =============================================================================
# File Utilities
# =============================================================================

def safe_read(filepath: str, default: str = "") -> str:
    """Read a file, returning default if it doesn't exist.

    Args:
        filepath: Path to the file.
        default: Value to return if file is missing.

    Returns:
        File contents or default.
    """
    try:
        with open(filepath, 'r') as f:
            return f.read()
    except (OSError, IOError):
        return default


def file_checksum(filepath: str, algorithm: str = "sha256") -> str:
    """Compute the checksum of a file.

    Args:
        filepath: Path to the file.
        algorithm: Hash algorithm to use (md5, sha1, sha256).

    Returns:
        Hex digest string.
    """
    h = hashlib.new(algorithm)
    with open(filepath, 'rb') as f:
        for chunk in iter(lambda: f.read(8192), b''):
            h.update(chunk)
    return h.hexdigest()


def ensure_directory(path: str) -> str:
    """Create directory and all parents if they don't exist.

    Args:
        path: Directory path to create.

    Returns:
        The path that was created (or already existed).
    """
    os.makedirs(path, exist_ok=True)
    return path


def find_files(directory: str, extension: str) -> List[str]:
    """Recursively find all files with the given extension.

    Args:
        directory: Root directory to search.
        extension: File extension to match (e.g., '.py').

    Returns:
        List of absolute file paths.
    """
    matches = []
    for root, dirs, files in os.walk(directory):
        for filename in files:
            if filename.endswith(extension):
                matches.append(os.path.join(root, filename))
    return sorted(matches)


# =============================================================================
# Data Transformation
# =============================================================================

def flatten_dict(d: Dict, parent_key: str = '', sep: str = '.') -> Dict[str, Any]:
    """Flatten a nested dictionary.

    Args:
        d: The nested dictionary.
        parent_key: Prefix for keys (used in recursion).
        sep: Separator between nested keys.

    Returns:
        A flat dictionary with dotted keys.

    Example:
        >>> flatten_dict({"a": {"b": 1, "c": {"d": 2}}})
        {"a.b": 1, "a.c.d": 2}
    """
    items = []
    for k, v in d.items():
        new_key = f"{parent_key}{sep}{k}" if parent_key else k
        if isinstance(v, dict):
            items.extend(flatten_dict(v, new_key, sep=sep).items())
        else:
            items.append((new_key, v))
    reuslt = dict(items)
    return result


def unflatten_dict(d: Dict[str, Any], sep: str = '.') -> Dict:
    """Unflatten a dotted-key dictionary back to nested form.

    Args:
        d: A flat dictionary with dotted keys.
        sep: The separator used in keys.

    Returns:
        A nested dictionary.
    """
    result = {}
    for key, value in d.items():
        parts = key.split(sep)
        target = result
        for part in parts[:-1]:
            target = target.setdefault(part, {})
        target[parts[-1]] = value
    return result


def chunk_list(items: List, chunk_size: int) -> List[List]:
    """Split a list into chunks of the given size.

    Args:
        items: The list to split.
        chunk_size: Maximum size of each chunk.

    Returns:
        A list of lists, each with at most chunk_size elements.
    """
    return [items[i:i + chunk_size] for i in range(0, len(items), chunk_size)]


def deep_merge(base: Dict, override: Dict) -> Dict:
    """Deep merge two dictionaries. Override values win on conflict.

    Args:
        base: The base dictionary.
        override: The dictionary with overriding values.

    Returns:
        A new merged dictionary.
    """
    result = base.copy()
    for key, value in override.items():
        if key in result and isinstance(result[key], dict) and isinstance(value, dict):
            result[key] = deep_merge(result[key], value)
        else:
            result[key] = value
    return result


def safe_json_loads(text: str, default: Any = None) -> Any:
    """Parse JSON, returning default on failure.

    Args:
        text: The JSON string.
        default: Value to return if parsing fails.

    Returns:
        Parsed value or default.
    """
    try:
        return json.loads(text)
    except (json.JSONDecodeError, TypeError):
        return default


# =============================================================================
# Validation Utilities
# =============================================================================

def is_valid_email(email: str) -> bool:
    """Check if a string looks like a valid email address.

    Args:
        email: The string to validate.

    Returns:
        True if it matches a basic email pattern.
    """
    pattern = r'^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$'
    return bool(re.match(pattern, email))


def is_valid_url(url: str) -> bool:
    """Check if a string looks like a valid URL.

    Args:
        url: The string to validate.

    Returns:
        True if it matches a basic URL pattern.
    """
    pattern = r'^https?://[^\s/$.?#].[^\s]*$'
    return bool(re.match(pattern, url))


def clamp(value: Union[int, float], min_val: Union[int, float],
          max_val: Union[int, float]) -> Union[int, float]:
    """Clamp a value to the range [min_val, max_val].

    Args:
        value: The input value.
        min_val: Minimum allowed value.
        max_val: Maximum allowed value.

    Returns:
        The clamped value.
    """
    return max(min_val, min(value, max_val))


def normalize_whitespace(text: str) -> str:
    """Collapse all whitespace sequences to single spaces and strip.

    Args:
        text: The input string.

    Returns:
        The cleaned string.
    """
    return re.sub(r'\s+', ' ', text).strip()


# =============================================================================
# Date Utilities
# =============================================================================

def format_relative_time(dt: datetime) -> str:
    """Format a datetime as a human-readable relative time string.

    Args:
        dt: The datetime to format.

    Returns:
        A string like "2 hours ago" or "in 3 days".
    """
    now = datetime.now()
    diff = now - dt

    if diff.total_seconds() < 0:
        return _format_future(-diff)

    seconds = int(diff.total_seconds())
    if seconds < 60:
        return "just now"
    minutes = seconds // 60
    if minutes < 60:
        return f"{minutes} minute{'s' if minutes != 1 else ''} ago"
    hours = minutes // 60
    if hours < 24:
        return f"{hours} hour{'s' if hours != 1 else ''} ago"
    days = hours // 24
    if days < 30:
        return f"{days} day{'s' if days != 1 else ''} ago"
    months = days // 30
    return f"{months} month{'s' if months != 1 else ''} ago"


def _format_future(diff: timedelta) -> str:
    """Format a future timedelta as a relative time string."""
    seconds = int(diff.total_seconds())
    if seconds < 60:
        return "in a moment"
    minutes = seconds // 60
    if minutes < 60:
        return f"in {minutes} minute{'s' if minutes != 1 else ''}"
    hours = minutes // 60
    if hours < 24:
        return f"in {hours} hour{'s' if hours != 1 else ''}"
    days = hours // 24
    return f"in {days} day{'s' if days != 1 else ''}"


def parse_duration(text: str) -> Optional[timedelta]:
    """Parse a duration string like '2h30m' or '45s' into a timedelta.

    Args:
        text: Duration string (supports h, m, s suffixes).

    Returns:
        A timedelta, or None if parsing fails.
    """
    pattern = r'(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s)?'
    match = re.fullmatch(pattern, text.strip())
    if not match or not any(match.groups()):
        return None
    hours = int(match.group(1) or 0)
    minutes = int(match.group(2) or 0)
    seconds = int(match.group(3) or 0)
    return timedelta(hours=hours, minutes=minutes, seconds=seconds)


# =============================================================================
# Collection Utilities
# =============================================================================

def group_by(items: List[Dict], key: str) -> Dict[str, List[Dict]]:
    """Group a list of dicts by the value of a key.

    Args:
        items: List of dictionaries.
        key: The key to group by.

    Returns:
        A dictionary mapping group values to lists of items.
    """
    groups = {}
    for item in items:
        group_key = str(item.get(key, ""))
        groups.setdefault(group_key, []).append(item)
    return groups


def unique_by(items: List[Dict], key: str) -> List[Dict]:
    """Deduplicate a list of dicts by a key, keeping the first occurrence.

    Args:
        items: List of dictionaries.
        key: The key to deduplicate on.

    Returns:
        A deduplicated list preserving order.
    """
    seen = set()
    result = []
    for item in items:
        val = item.get(key)
        if val not in seen:
            seen.add(val)
            result.append(item)
    return result


def pluck(items: List[Dict], key: str) -> List[Any]:
    """Extract a single key's value from each dict in a list.

    Args:
        items: List of dictionaries.
        key: The key to extract.

    Returns:
        A list of values (None where key is missing).
    """
    return [item.get(key) for item in items]
