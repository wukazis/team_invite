#!/usr/bin/env python3
import json
import sys
from typing import Any, Dict

import requests


def build_headers(account_id: str, auth_token: str) -> Dict[str, str]:
    token = auth_token if auth_token.startswith("Bearer") else f"Bearer {auth_token}"
    return {
        "accept": "*/*",
        "accept-language": "zh-CN,zh;q=0.9",
        "authorization": token,
        "chatgpt-account-id": account_id,
        "user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
    }


def fetch_status(account_id: str, auth_token: str) -> Dict[str, Any]:
    headers = build_headers(account_id, auth_token)
    subs_url = f"https://chatgpt.com/backend-api/subscriptions?account_id={account_id}"
    invites_url = f"https://chatgpt.com/backend-api/accounts/{account_id}/invites?offset=0&limit=1&query="

    subs_resp = requests.get(subs_url, headers=headers, timeout=15)
    subs_resp.raise_for_status()
    subs_data = subs_resp.json()

    invites_resp = requests.get(invites_url, headers=headers, timeout=15)
    invites_resp.raise_for_status()
    invites_data = invites_resp.json()

    return {
        "status": "ok",
        "seats_in_use": subs_data.get("seats_in_use"),
        "seats_entitled": subs_data.get("seats_entitled"),
        "pending_invites": invites_data.get("total"),
        "plan_type": subs_data.get("plan_type"),
        "active_start": subs_data.get("active_start"),
        "active_until": subs_data.get("active_until"),
        "billing_period": subs_data.get("billing_period"),
        "billing_currency": subs_data.get("billing_currency"),
        "will_renew": subs_data.get("will_renew"),
        "is_delinquent": subs_data.get("is_delinquent"),
    }


def main():
    if len(sys.argv) != 3:
        print(json.dumps({"error": "Usage: team_status.py <account_id> <auth_token>"}))
        sys.exit(1)

    account_id, auth_token = sys.argv[1], sys.argv[2]
    try:
        data = fetch_status(account_id, auth_token)
        print(json.dumps(data))
        sys.exit(0)
    except requests.HTTPError as e:
        resp = e.response
        print(json.dumps({
            "error": str(e),
            "status": resp.status_code if resp else None,
            "body": resp.text if resp else "",
        }))
        sys.exit(1)
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.exit(1)


if __name__ == "__main__":
    main()
