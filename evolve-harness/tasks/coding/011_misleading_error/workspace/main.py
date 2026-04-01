"""Main entry point for geometry calculations.

This file is CORRECT — it imports from utils and config, then runs
a series of geometry calculations and prints results.
"""

import sys
import os

# Ensure local imports work
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from utils import distance, midpoint, triangle_area, hypotenuse, normalize_vector
from config import DECIMAL_PLACES, OPERATIONS, SHOW_STEPS


def format_point(x, y):
    """Format a point as a readable string."""
    return f"({x:.{DECIMAL_PLACES}f}, {y:.{DECIMAL_PLACES}f})"


def run_calculations():
    """Run a series of geometry calculations and print results."""
    print("Geometry Calculator")
    print("=" * 40)

    # Define test points
    points = [
        (0, 0, 3, 4),
        (1, 1, 4, 5),
        (-2, 3, 5, -1),
    ]

    # Distance calculations
    if "distance" in OPERATIONS:
        print("\nDistances:")
        for x1, y1, x2, y2 in points:
            d = distance(x1, y1, x2, y2)
            p1 = format_point(x1, y1)
            p2 = format_point(x2, y2)
            print(f"  {p1} to {p2} = {d:.{DECIMAL_PLACES}f}")
            if SHOW_STEPS:
                print(f"    dx={x2-x1}, dy={y2-y1}")

    # Midpoint calculations
    if "midpoint" in OPERATIONS:
        print("\nMidpoints:")
        for x1, y1, x2, y2 in points:
            mx, my = midpoint(x1, y1, x2, y2)
            p1 = format_point(x1, y1)
            p2 = format_point(x2, y2)
            mp = format_point(mx, my)
            print(f"  {p1} to {p2} -> midpoint {mp}")

    # Area calculation
    if "area" in OPERATIONS:
        print("\nTriangle Area:")
        area = triangle_area(0, 0, 4, 0, 0, 3)
        print(f"  Triangle (0,0)-(4,0)-(0,3): area = {area:.{DECIMAL_PLACES}f}")

    # Hypotenuse calculation
    if "hypotenuse" in OPERATIONS:
        print("\nHypotenuse:")
        h = hypotenuse(3, 4)
        print(f"  legs 3, 4 -> hypotenuse = {h:.{DECIMAL_PLACES}f}")

    # Vector normalization
    if "normalize" in OPERATIONS:
        print("\nNormalized Vectors:")
        vectors = [(3, 4), (1, 0), (5, 12)]
        for x, y in vectors:
            nx, ny = normalize_vector(x, y)
            print(f"  ({x}, {y}) -> {format_point(nx, ny)}")

    print("\nDone.")


if __name__ == "__main__":
    run_calculations()
