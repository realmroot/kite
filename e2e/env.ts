import { mkdirSync } from "node:fs";
import { dirname, resolve } from "node:path";
import { fileURLToPath } from "node:url";

const e2eDir = dirname(fileURLToPath(import.meta.url));

export const authFile = resolve(e2eDir, ".auth", "admin.json");
export const oidcIssuer =
  process.env.KITE_E2E_OIDC_ISSUER || "https://localhost:5556";

export const adminUser = {
  username: process.env.KITE_E2E_ADMIN_USERNAME || "admin@kite.test",
  name: process.env.KITE_E2E_ADMIN_NAME || "Kite Admin",
  password: process.env.KITE_E2E_ADMIN_PASSWORD || "KiteE2E!2345",
  subject:
    process.env.KITE_E2E_ADMIN_SUBJECT ||
    "CiQxMTExMTExMS0xMTExLTQxMTEtODExMS0xMTExMTExMTExMTESBWxvY2Fs",
};

export const viewerUser = {
  username: process.env.KITE_E2E_VIEWER_USERNAME || "viewer@kite.test",
  name: process.env.KITE_E2E_VIEWER_NAME || "Kite Viewer",
  password: process.env.KITE_E2E_VIEWER_PASSWORD || "KiteViewer!2345",
  subject:
    process.env.KITE_E2E_VIEWER_SUBJECT ||
    "CiQyMjIyMjIyMi0yMjIyLTQyMjItODIyMi0yMjIyMjIyMjIyMjISBWxvY2Fs",
};

export const kindClusterName = process.env.KITE_E2E_CLUSTER_NAME || "kite-e2e";

export function ensureAuthDir() {
  mkdirSync(dirname(authFile), { recursive: true });
}
