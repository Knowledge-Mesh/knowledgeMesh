"""Minimal CLI: km-chat "prompt" """

from __future__ import annotations

import argparse
import json
import os
import sys

from .client import KM, KMError


def main() -> None:
    parser = argparse.ArgumentParser(
        prog="km-chat",
        description="Send a prompt to the KnowledgeMesh network",
    )
    parser.add_argument("prompt", help="The prompt to send")
    parser.add_argument(
        "--secret",
        default=os.environ.get("KM_SECRET"),
        help="Node secret (or set KM_SECRET env var)",
    )
    parser.add_argument(
        "--broker",
        default=os.environ.get("KM_BROKER_URL", "https://km-broker.onrender.com"),
        help="Broker URL",
    )
    parser.add_argument("--model", default=None, help="Request a specific model")
    parser.add_argument("--tier", default=None, help="Prefer a tier: api, subscription")
    parser.add_argument("--json", action="store_true", dest="output_json", help="Output raw JSON")

    args = parser.parse_args()

    if not args.secret:
        print("Error: No secret. Use --secret or set KM_SECRET.", file=sys.stderr)
        sys.exit(1)

    try:
        km = KM(secret=args.secret, broker_url=args.broker)
        result = km.chat(args.prompt, model=args.model, tier=args.tier)
    except KMError as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)

    if args.output_json:
        print(json.dumps(result, indent=2))
    else:
        print(result["content"])
        print(
            f"\n--- {result['tokens']} tokens | "
            f"${result['cost']:.6f} | "
            f"saved {result['savings']:.0f}% vs API | "
            f"via {result['worker']} ---",
            file=sys.stderr,
        )


if __name__ == "__main__":
    main()
