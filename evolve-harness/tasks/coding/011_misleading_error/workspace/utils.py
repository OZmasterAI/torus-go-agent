"""Geometry utility functions."""

import math
from config import DEFAULT_PRECISION, CONVERGENCE_THRESHOLD


def distance(x1, y1, x2, y2):
    """Calculate Euclidean distance between two points.

    Args:
        x1, y1: Coordinates of first point.
        x2, y2: Coordinates of second point.

    Returns:
        The distance as a float.
    """
    dx = x2 - x1
    dy = y2 - y1
    # BUG: math.sqr does not exist — should be math.sqrt
    return math.sqr(dx * dx + dy * dy)


def midpoint(x1, y1, x2, y2):
    """Calculate the midpoint between two points.

    Args:
        x1, y1: Coordinates of first point.
        x2, y2: Coordinates of second point.

    Returns:
        A tuple (mx, my) of the midpoint coordinates.
    """
    return ((x1 + x2) / 2, (y1 + y2) / 2)


def triangle_area(x1, y1, x2, y2, x3, y3):
    """Calculate the area of a triangle given three vertices.

    Uses the shoelace formula.

    Args:
        x1, y1: First vertex.
        x2, y2: Second vertex.
        x3, y3: Third vertex.

    Returns:
        The area as a positive float.
    """
    return abs((x1 * (y2 - y3) + x2 * (y3 - y1) + x3 * (y1 - y2)) / 2.0)


def hypotenuse(a, b):
    """Calculate the hypotenuse of a right triangle.

    Args:
        a: Length of one leg.
        b: Length of the other leg.

    Returns:
        The hypotenuse length.
    """
    return math.sqrt(a * a + b * b)


def normalize_vector(x, y):
    """Normalize a 2D vector to unit length.

    Args:
        x: X component.
        y: Y component.

    Returns:
        A tuple (nx, ny) of the normalized vector.

    Raises:
        ValueError: If the vector has zero length.
    """
    length = math.sqrt(x * x + y * y)
    if length < CONVERGENCE_THRESHOLD:
        raise ValueError("Cannot normalize a zero-length vector")
    return (x / length, y / length)


def round_result(value, precision=None):
    """Round a result to the configured precision.

    Args:
        value: The number to round.
        precision: Override for decimal places (uses DEFAULT_PRECISION if None).

    Returns:
        The rounded value.
    """
    if precision is None:
        precision = DEFAULT_PRECISION
    return round(value, precision)
