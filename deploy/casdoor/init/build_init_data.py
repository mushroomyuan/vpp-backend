#!/usr/bin/env python3
"""Rebuild conf/init_data.json from conf/certs/jwt.{crt,key}.

Dev-only seed for VPP Casdoor (org default, app vpp-resource, roles, users,
authz Model + Permissions for C5).
Run from repo:  python3 deploy/casdoor/init/build_init_data.py
"""
from __future__ import annotations

import json
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
CERT_PATH = ROOT / "conf" / "certs" / "jwt.crt"
KEY_PATH = ROOT / "conf" / "certs" / "jwt.key"
MODEL_PATH = ROOT / "conf" / "authz_model.conf"
OUT_PATH = ROOT / "conf" / "init_data.json"

# Catalog objs used by Permission.Resources (see docs/AUTHZ_CENTRALIZATION_PLAN.md §7.1).
# Wildcard resource:* matches any resource:{name} via keyMatch2 in authz_model.conf.
AUTHZ_MODEL_NAME = "vpp-rbac"
AUTHZ_RESOURCES = ["resource:*"]
DISPATCH_RESOURCES = ["dispatch:tasks"]
GATEWAY_RESOURCES = ["gateway:mappings"]
TELEMETRY_RESOURCES = [
    "telemetry:telemetry",
    "telemetry:snapshots",
    "telemetry:aggregation",
]
CREATED = "2026-01-01T00:00:00Z"

ACCOUNT_ITEMS = [
    {"name": "Organization", "visible": True, "viewRule": "Public", "modifyRule": "Admin"},
    {"name": "ID", "visible": True, "viewRule": "Public", "modifyRule": "Immutable"},
    {"name": "Name", "visible": True, "viewRule": "Public", "modifyRule": "Admin"},
    {"name": "Display name", "visible": True, "viewRule": "Public", "modifyRule": "Self"},
    {"name": "Avatar", "visible": True, "viewRule": "Public", "modifyRule": "Self"},
    {"name": "User type", "visible": True, "viewRule": "Public", "modifyRule": "Admin"},
    {"name": "Password", "visible": True, "viewRule": "Self", "modifyRule": "Self"},
    {"name": "Email", "visible": True, "viewRule": "Public", "modifyRule": "Self"},
    {"name": "Phone", "visible": True, "viewRule": "Public", "modifyRule": "Self"},
    {"name": "Roles", "visible": True, "viewRule": "Public", "modifyRule": "Immutable"},
    {"name": "Permissions", "visible": True, "viewRule": "Public", "modifyRule": "Immutable"},
    {"name": "Is admin", "visible": True, "viewRule": "Admin", "modifyRule": "Admin"},
    {"name": "Is forbidden", "visible": True, "viewRule": "Admin", "modifyRule": "Admin"},
    {"name": "Is deleted", "visible": True, "viewRule": "Admin", "modifyRule": "Admin"},
]


def _permission(
    name: str,
    display: str,
    description: str,
    roles: list[str],
    actions: list[str],
    resources: list[str] | None = None,
) -> dict:
    return {
        "owner": "default",
        "name": name,
        "createdTime": CREATED,
        "displayName": display,
        "description": description,
        "users": [],
        "groups": [],
        "roles": roles,
        "domains": [],
        "model": f"default/{AUTHZ_MODEL_NAME}",
        "adapter": "",
        "resourceType": "Custom",
        "resources": list(resources if resources is not None else AUTHZ_RESOURCES),
        "actions": actions,
        "effect": "Allow",
        "isEnabled": True,
        "submitter": "admin",
        "approver": "admin",
        "approveTime": CREATED,
        "state": "Approved",
    }


def main() -> int:
    if not CERT_PATH.is_file() or not KEY_PATH.is_file():
        print(f"ERROR: missing {CERT_PATH} or {KEY_PATH}", file=sys.stderr)
        print("Generate with:", file=sys.stderr)
        print(
            "  openssl genrsa -out deploy/casdoor/conf/certs/jwt.key 2048 && "
            "openssl req -new -x509 -key deploy/casdoor/conf/certs/jwt.key "
            "-out deploy/casdoor/conf/certs/jwt.crt -days 3650 -subj '/CN=vpp-casdoor-dev'",
            file=sys.stderr,
        )
        return 1
    if not MODEL_PATH.is_file():
        print(f"ERROR: missing {MODEL_PATH}", file=sys.stderr)
        return 1

    cert = CERT_PATH.read_text()
    key = KEY_PATH.read_text()
    model_text = MODEL_PATH.read_text().strip() + "\n"

    # C3-equivalent role bindings (placeholder roles). Fine-grained splits stay in Casdoor.
    # C10a: dispatch control-class bindings (dispatch:tasks).
    permissions = [
        _permission(
            "vpp-resource-read",
            "VPP Resource Read",
            "viewer/operator/admin: catalog read (maps from GET/HEAD)",
            ["default/viewer", "default/operator", "default/admin"],
            ["read"],
        ),
        _permission(
            "vpp-resource-write",
            "VPP Resource Write",
            "operator/admin: non-destructive write (POST/PUT/PATCH except change-lifecycle)",
            ["default/operator", "default/admin"],
            ["write"],
        ),
        _permission(
            "vpp-resource-admin",
            "VPP Resource Admin",
            "admin only: delete + change-lifecycle",
            ["default/admin"],
            ["delete", "change-lifecycle"],
        ),
        _permission(
            "vpp-dispatch-read",
            "VPP Dispatch Read",
            "viewer/operator/admin: GetTask",
            ["default/viewer", "default/operator", "default/admin"],
            ["read"],
            DISPATCH_RESOURCES,
        ),
        _permission(
            "vpp-dispatch-submit",
            "VPP Dispatch Submit",
            "operator/admin: SubmitTask (control commands)",
            ["default/operator", "default/admin"],
            ["submit"],
            DISPATCH_RESOURCES,
        ),
        _permission(
            "vpp-dispatch-cancel",
            "VPP Dispatch Cancel",
            "admin only: CancelTask",
            ["default/admin"],
            ["cancel"],
            DISPATCH_RESOURCES,
        ),
        _permission(
            "vpp-gateway-mappings-read",
            "VPP Gateway Mappings Read",
            "viewer/operator/admin: list mappings",
            ["default/viewer", "default/operator", "default/admin"],
            ["read"],
            GATEWAY_RESOURCES,
        ),
        _permission(
            "vpp-gateway-mappings-write",
            "VPP Gateway Mappings Write",
            "operator/admin: create/disable mappings",
            ["default/operator", "default/admin"],
            ["write"],
            GATEWAY_RESOURCES,
        ),
        _permission(
            "vpp-gateway-mappings-delete",
            "VPP Gateway Mappings Delete",
            "admin only: delete mappings",
            ["default/admin"],
            ["delete"],
            GATEWAY_RESOURCES,
        ),
        _permission(
            "vpp-telemetry-read",
            "VPP Telemetry Read",
            "viewer/operator/admin: QueryTelemetry/GetSnapshot/GetFleetSnapshot/QueryAggregation",
            ["default/viewer", "default/operator", "default/admin"],
            ["read"],
            TELEMETRY_RESOURCES,
        ),
    ]

    data = {
        "organizations": [
            {
                "owner": "admin",
                "name": "default",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "VPP Default Tenant",
                "websiteUrl": "http://127.0.0.1:8000",
                "favicon": "",
                "passwordType": "bcrypt",
                "passwordSalt": "",
                "passwordOptions": ["AtLeast6"],
                "countryCodes": ["CN", "US"],
                "defaultAvatar": "",
                "defaultApplication": "vpp-resource",
                "tags": [],
                "languages": ["en", "zh"],
                "masterPassword": "",
                "defaultPassword": "",
                "initScore": 2000,
                "enableSoftDeletion": False,
                "isProfilePublic": True,
                "disableSignin": False,
                "accountItems": ACCOUNT_ITEMS,
            }
        ],
        "certs": [
            {
                "owner": "admin",
                "name": "cert-vpp",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "VPP DEV JWT Cert (fixed)",
                "scope": "JWT",
                "type": "x509",
                "cryptoAlgorithm": "RS256",
                "bitSize": 2048,
                "expireInYears": 10,
                "certificate": cert,
                "privateKey": key,
            }
        ],
        "applications": [
            {
                "owner": "admin",
                "name": "vpp-resource",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "VPP Resource Admin API",
                "logo": "https://cdn.casbin.org/img/casdoor-logo_1185x256.png",
                "homepageUrl": "http://127.0.0.1:9080/resource",
                "description": "Dev OIDC app for /resource/* via APISIX",
                "organization": "default",
                "cert": "cert-vpp",
                "enablePassword": True,
                "enableSignUp": False,
                "disableSignin": False,
                "clientId": "vpp-resource-dev-client",
                "clientSecret": "vpp-resource-dev-secret",
                "providers": [],
                "signinMethods": [
                    {"name": "Password", "displayName": "Password", "rule": "All"}
                ],
                "signupItems": [
                    {
                        "name": "ID",
                        "visible": False,
                        "required": True,
                        "prompted": False,
                        "rule": "Random",
                    },
                    {
                        "name": "Username",
                        "visible": True,
                        "required": True,
                        "prompted": False,
                        "rule": "None",
                    },
                    {
                        "name": "Password",
                        "visible": True,
                        "required": True,
                        "prompted": False,
                        "rule": "None",
                    },
                    {
                        "name": "Confirm password",
                        "visible": True,
                        "required": True,
                        "prompted": False,
                        "rule": "None",
                    },
                ],
                "grantTypes": [
                    "authorization_code",
                    "password",
                    "client_credentials",
                    "refresh_token",
                ],
                "redirectUris": [
                    "http://127.0.0.1:9080/resource/",
                    "http://localhost:9080/resource/",
                ],
                "tokenFormat": "JWT",
                "tokenFields": [],
                "expireInHours": 24,
                "refreshExpireInHours": 168,
                "failedSigninLimit": 20,
                "failedSigninFrozenTime": 15,
            }
        ],
        "users": [
            {
                "owner": "default",
                "name": "admin",
                "createdTime": "2026-01-01T00:00:00Z",
                "type": "normal-user",
                "password": "vpp-admin-dev",
                "displayName": "VPP Admin",
                "avatar": "",
                "email": "admin@default.local",
                "phone": "",
                "countryCode": "CN",
                "address": [],
                "affiliation": "",
                "tag": "",
                "score": 2000,
                "ranking": 1,
                "isAdmin": True,
                "isForbidden": False,
                "isDeleted": False,
                "signupApplication": "vpp-resource",
            },
            {
                "owner": "default",
                "name": "operator",
                "createdTime": "2026-01-01T00:00:00Z",
                "type": "normal-user",
                "password": "vpp-operator-dev",
                "displayName": "VPP Operator",
                "avatar": "",
                "email": "operator@default.local",
                "phone": "",
                "countryCode": "CN",
                "address": [],
                "affiliation": "",
                "tag": "",
                "score": 2000,
                "ranking": 2,
                "isAdmin": False,
                "isForbidden": False,
                "isDeleted": False,
                "signupApplication": "vpp-resource",
            },
            {
                "owner": "default",
                "name": "viewer",
                "createdTime": "2026-01-01T00:00:00Z",
                "type": "normal-user",
                "password": "vpp-viewer-dev",
                "displayName": "VPP Viewer",
                "avatar": "",
                "email": "viewer@default.local",
                "phone": "",
                "countryCode": "CN",
                "address": [],
                "affiliation": "",
                "tag": "",
                "score": 2000,
                "ranking": 3,
                "isAdmin": False,
                "isForbidden": False,
                "isDeleted": False,
                "signupApplication": "vpp-resource",
            },
        ],
        "roles": [
            {
                "owner": "default",
                "name": "admin",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "Admin",
                "description": "Full Resource API access",
                "users": ["default/admin"],
                "groups": [],
                "roles": [],
                "isEnabled": True,
            },
            {
                "owner": "default",
                "name": "operator",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "Operator",
                "description": "Read/write Resource API (no delete/lifecycle)",
                "users": ["default/operator"],
                "groups": [],
                "roles": [],
                "isEnabled": True,
            },
            {
                "owner": "default",
                "name": "viewer",
                "createdTime": "2026-01-01T00:00:00Z",
                "displayName": "Viewer",
                "description": "Read-only Resource API",
                "users": ["default/viewer"],
                "groups": [],
                "roles": [],
                "isEnabled": True,
            },
        ],
        "providers": [],
        "ldaps": [],
        "models": [
            {
                "owner": "default",
                "name": AUTHZ_MODEL_NAME,
                "createdTime": CREATED,
                "displayName": "VPP RBAC",
                "description": "Shared Casbin model for local PDP (AUTHZ C5); see conf/authz_model.conf",
                "modelText": model_text,
            }
        ],
        "permissions": permissions,
        "payments": [],
        "products": [],
        "resources": [],
        "syncers": [],
        "tokens": [],
        "webhooks": [],
        "groups": [],
        "adapters": [],
        "enforcers": [],
        "plans": [],
        "pricings": [],
    }

    OUT_PATH.write_text(json.dumps(data, indent=2) + "\n")
    print(f"wrote {OUT_PATH} ({OUT_PATH.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
