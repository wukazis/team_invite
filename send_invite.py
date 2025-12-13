
#!/usr/bin/env python3

import requests

import sys

import json

def send_invite(account_id, auth_token, email):

    url = f"https://chatgpt.com/backend-api/accounts/{account_id}/invites"

    headers = {

        "accept": "*/*",

        "authorization": f"Bearer {auth_token}" if not auth_token.startswith("Bearer") else auth_token,

        "chatgpt-account-id": account_id,

        "content-type": "application/json",

        "origin": "https://chatgpt.com",

        "referer": "https://chatgpt.com/",

        "user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36",

    }

    payload = {"email_addresses": [email], "role": "standard-user", "resend_emails": True}

    resp = requests.post(url, headers=headers, json=payload, timeout=15)

    print(json.dumps({"status": resp.status_code, "body": resp.text}))

    return resp.status_code == 200

if __name__ == "__main__":

    if len(sys.argv) != 4:

        print(json.dumps({"error": "Usage: send_invite.py <account_id> <auth_token> <email>"}))

        sys.exit(1)

    success = send_invite(sys.argv[1], sys.argv[2], sys.argv[3])

    sys.exit(0 if success else 1)

