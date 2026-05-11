import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { describe, expect, it } from "vitest";

const tlsConfigPath = resolve(__dirname, "../../../deploy/nginx/tls.conf");

function readTlsConfig() {
  return readFileSync(tlsConfigPath, "utf8");
}

describe("mobile entry redirect", () => {
  it("redirects phone clients from the production root to the mobile entry", () => {
    const config = readTlsConfig();

    expect(config).toMatch(/map\s+\$http_user_agent\s+\$is_mobile_client\s*{/);
    expect(config).toMatch(/location\s+=\s+\/\s*{[^}]*return\s+302\s+\/m;/s);
  });

  it("keeps tablet clients out of the automatic phone redirect", () => {
    const config = readTlsConfig();

    expect(config).toMatch(/~\*ipad\s+0;/i);
    expect(config).toMatch(/~\*tablet\s+0;/i);
    expect(config).not.toMatch(/\|\s*mobile[)"|]/i);
  });
});
