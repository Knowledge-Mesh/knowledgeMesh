"""KnowledgeMesh client — buy AI inference from the peer-to-peer network."""

from __future__ import annotations

import requests
from typing import Any, Dict, List, Optional


class KMError(Exception):
    """Raised when a KnowledgeMesh API call fails."""
    pass


class KM:
    """KnowledgeMesh client.

    Usage::

        from knowledgemesh import KM

        km = KM(secret="km-sec-xxx")
        result = km.chat("What is 2+2?")
        print(result["content"])  # "4"
    """

    def __init__(
        self,
        secret: str,
        broker_url: str = "https://km-broker.onrender.com",
        buyer: Optional[str] = None,
    ):
        self.secret = secret
        self.broker_url = broker_url.rstrip("/")
        self._session = requests.Session()

        # Auto-discover buyer name from secret
        if buyer:
            self.buyer = buyer
        else:
            self.buyer = self._fetch_buyer_name()

    def _parse_response(self, resp):
        """Parse a broker response, raising KMError on non-JSON or error status."""
        try:
            data = resp.json()
        except (ValueError, requests.exceptions.JSONDecodeError):
            raise KMError(f"Broker returned non-JSON response (HTTP {resp.status_code}): {resp.text[:200]}")
        if resp.status_code != 200:
            error_msg = data.get("error", data.get("message", f"HTTP {resp.status_code}"))
            raise KMError(error_msg)
        return data

    def _fetch_buyer_name(self) -> str:
        """Get the buyer's node name from the broker using the secret."""
        try:
            resp = self._session.get(
                f"{self.broker_url}/node-config",
                headers={"Authorization": f"Bearer {self.secret}"},
                timeout=10,
            )
            data = self._parse_response(resp)
            return data["name"]
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker at {self.broker_url}: {e}")

    def chat(
        self,
        prompt: str,
        model: Optional[str] = None,
        tier: Optional[str] = None,
        max_budget: float = 5.0,
    ) -> Dict[str, Any]:
        """Send a single prompt and get a response.

        Returns::

            {
                "content": "The answer is ...",
                "tokens": 42,
                "cost": 0.000021,
                "api_cost": 0.00063,
                "savings": 96.7,
                "worker": "some-node",
                "task_id": "task-abc123",
                "model": "claude-sonnet-4-20250514",
            }
        """
        return self.chat_messages(
            messages=[{"role": "user", "content": prompt}],
            model=model,
            tier=tier,
            max_budget=max_budget,
        )

    def chat_messages(
        self,
        messages: List[Dict[str, str]],
        model: Optional[str] = None,
        tier: Optional[str] = None,
        max_budget: float = 5.0,
    ) -> Dict[str, Any]:
        """Send a conversation (list of messages) and get a response."""
        payload: Dict[str, Any] = {
            "buyer": self.buyer,
            "buyer_secret": self.secret,
            "messages": messages,
            "max_budget": max_budget,
        }
        if model:
            payload["model"] = model
        if tier:
            payload["tier_preference"] = tier

        try:
            resp = self._session.post(
                f"{self.broker_url}/task",
                json=payload,
                timeout=60,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        data = self._parse_response(resp)

        result = data.get("result", {})
        choices = result.get("choices", [])
        content = choices[0]["message"]["content"] if choices else ""
        usage = result.get("usage", {})

        return {
            "content": content,
            "tokens": usage.get("total_tokens", 0),
            "cost": data.get("credits_charged", 0),
            "api_cost": data.get("api_cost", 0),
            "savings": data.get("savings_percent", 0),
            "worker": data.get("worker_name", ""),
            "task_id": data.get("task_id", ""),
            "model": result.get("model", ""),
            "model_verified": data.get("model_verified", False),
            "tier": result.get("tier", ""),
            "raw": data,
        }

    def balance(self) -> float:
        """Get current credit balance."""
        info = self.whoami()
        return info.get("credits", 0.0)

    def status(self) -> Dict[str, Any]:
        """Get network status."""
        try:
            resp = self._session.get(f"{self.broker_url}/status", timeout=10)
            return self._parse_response(resp)
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

    def nodes(self) -> List[Dict[str, Any]]:
        """Get list of online nodes."""
        data = self.status()
        return [n for n in data.get("nodes", []) if n.get("status") == "online"]

    # ── Account & discovery ───────────────────────────────────────────

    def whoami(self) -> Dict[str, Any]:
        """Get account info for the current secret.

        Returns::

            {
                "name": "my-node",
                "credits": 12.5,
                "tier": "pro",
                "email_registered": True,
                ...
            }
        """
        try:
            resp = self._session.get(
                f"{self.broker_url}/whoami",
                headers={"Authorization": f"Bearer {self.secret}"},
                timeout=10,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        return self._parse_response(resp)

    def models(self) -> List[Dict[str, Any]]:
        """Get list of available models with pricing.

        Returns a list of dicts, each containing model name, provider,
        and price information.
        """
        try:
            resp = self._session.get(
                f"{self.broker_url}/models",
                timeout=10,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        return self._parse_response(resp)

    def recover(self, name: str, email: str) -> Dict[str, Any]:
        """Initiate account recovery.

        Sends a recovery email to the registered address for the given
        node name.

        Args:
            name:  The node/account name to recover.
            email: The email address on file.

        Returns:
            Broker response dict (typically contains a status message).
        """
        try:
            resp = self._session.post(
                f"{self.broker_url}/recover",
                json={"name": name, "email": email},
                timeout=10,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        return self._parse_response(resp)

    def reset_secret(self, reset_token: str) -> Dict[str, Any]:
        """Exchange a reset token for a new secret.

        Args:
            reset_token: The token received via the recovery email.

        Returns:
            Dict containing the new secret, e.g. ``{"secret": "km-sec-..."}``
        """
        try:
            resp = self._session.post(
                f"{self.broker_url}/reset-secret",
                json={"reset_token": reset_token},
                timeout=10,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        return self._parse_response(resp)

    # ── OpenAI-compatible interface ──────────────────────────────────

    def completions_create(
        self,
        messages: List[Dict[str, str]],
        model: str = "claude-sonnet",
        **kwargs,
    ) -> Dict[str, Any]:
        """OpenAI-compatible chat completions call via the broker's /v1/chat/completions.

        Usage with frameworks::

            km = KM(secret="km-sec-xxx")
            resp = km.completions_create(
                messages=[{"role": "user", "content": "hi"}],
                model="claude-sonnet",
            )
            print(resp["choices"][0]["message"]["content"])
        """
        try:
            resp = self._session.post(
                f"{self.broker_url}/v1/chat/completions",
                json={"model": model, "messages": messages, **kwargs},
                headers={"Authorization": f"Bearer {self.secret}"},
                timeout=60,
            )
        except requests.RequestException as e:
            raise KMError(f"Cannot reach broker: {e}")

        return self._parse_response(resp)
