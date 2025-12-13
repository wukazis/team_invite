#!/usr/bin/env python3
import json
import os
import sys
import requests


ACCOUNT_ID = os.environ.get("ACCOUNT_ID")
AUTHORIZATION_TOKEN = os.environ.get("AUTHORIZATION_TOKEN")


def build_headers():
    return {
        "accept": "*/*",
        "accept-language": "zh-CN,zh;q=0.9",
        "authorization": AUTHORIZATION_TOKEN,
        "chatgpt-account-id": ACCOUNT_ID,
        "content-type": "application/json",
        "origin": "https://chatgpt.com",
        "referer": "https://chatgpt.com/",
        "sec-ch-ua": '"Chromium";v="135", "Not)A;Brand";v="99", "Google Chrome";v="135"',
        "sec-ch-ua-mobile": "?0",
        "sec-ch-ua-platform": '"Windows"',
        "user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",
    }


def send_invite(email: str):
    url = f"https://chatgpt.com/backend-api/accounts/{ACCOUNT_ID}/invites"
    payload = {
        "email_addresses": [email],
        "role": "standard-user",
        "resend_emails": True,
    }
    resp = requests.post(url, json=payload, headers=build_headers(), timeout=15)
    return resp.status_code, resp.text


def main():
    if not ACCOUNT_ID or not AUTHORIZATION_TOKEN:
        print(json.dumps({"status": 0, "body": "ACCOUNT_ID/AUTHORIZATION_TOKEN missing"}))
        return 2
    if len(sys.argv) < 2:
        print(json.dumps({"status": 0, "body": "email required"}))
        return 2
    email = sys.argv[1].strip()
    if not email:
        print(json.dumps({"status": 0, "body": "email required"}))
        return 2
    try:
        status, body = send_invite(email)
        print(json.dumps({"status": status, "body": body}))
        return 0 if 200 <= status < 300 else 1
    except Exception as exc:  # noqa: BLE001
        print(json.dumps({"status": 0, "body": str(exc)}))
        return 1


if __name__ == "__main__":
    sys.exit(main())
