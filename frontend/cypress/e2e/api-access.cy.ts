describe("API access — keys, roles, ACL", () => {
  const now = Date.now();
  const username = `e2e-api-${now}`;
  const password = "supersecret123";
  const slug = `e2e-api-pad-${now}`;
  const wildcardBase = `e2e-api-wild-${now}`;
  const wildcardSlug = `${wildcardBase}/inner`;
  const outsideSlug = `e2e-api-outside-${now}`;

  let token = "";
  let fullKey = "";
  let scopedKey = "";
  let viewerRoleId = "";

  const apiUrl = () => Cypress.env("apiUrl") as string;

  function visitAuthed(path: string) {
    cy.visit(path, {
      onBeforeLoad(win) {
        win.sessionStorage.setItem("session_token", token);
      },
    });
  }

  it("signs up a new account", () => {
    cy.visit("/_/login");
    cy.contains("button", "Sign up").click();
    cy.get('input[placeholder="alice"]').type(username);
    cy.get('input[placeholder="min 8 characters"]').type(password);
    cy.contains("button", "Create account").click();
    cy.contains("Account created!").should("be.visible");
    cy.contains("button", "Skip").click();
    cy.location("pathname").should("eq", "/");

    cy.window().then((win) => {
      token = win.sessionStorage.getItem("session_token") ?? "";
      expect(token, "session token").to.not.equal("");
    });
  });

  it("blocks API key creation on the free tier", () => {
    visitAuthed("/_/profile");
    cy.contains("button", "API").click();
    cy.get('input[placeholder="Key name"]').type("blocked-bot");
    cy.contains("button", "Create key").click();
    cy.contains("paid tier required").should("be.visible");
  });

  it("upgrades the account to paid tier (test-only DB task)", () => {
    cy.task("setUserTier", { username, tier: "paid" });
  });

  it("creates a full-access API key and reveals the raw key once", () => {
    visitAuthed("/_/profile");
    cy.contains("button", "API").click();
    cy.get('input[placeholder="Key name"]').type("full-bot");
    cy.contains("button", "Create key").click();

    cy.contains("Copy this key now").should("be.visible");
    cy.get("code")
      .invoke("text")
      .then((key) => {
        fullKey = key.trim();
        expect(fullKey.length).to.be.greaterThan(20);
      });
    cy.contains("full-bot").should("be.visible");
  });

  it("uses the full-access key to create, read, and delete its own pad", () => {
    cy.request({
      method: "POST",
      url: `${apiUrl()}/api/pads/${slug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
      body: { content: "hello from api key", encrypted: false },
    })
      .its("status")
      .should("eq", 200);

    cy.request({
      url: `${apiUrl()}/api/pads/${slug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
    }).then((res) => {
      expect(res.body.content).to.eq("hello from api key");
    });

    cy.request({
      method: "DELETE",
      url: `${apiUrl()}/api/pads/${slug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
    })
      .its("status")
      .should("eq", 200);

    cy.request({
      url: `${apiUrl()}/api/pads/${slug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
      failOnStatusCode: false,
    })
      .its("status")
      .should("eq", 404);
  });

  it("seeds pads for the ACL/wildcard scoping test", () => {
    cy.request({
      method: "POST",
      url: `${apiUrl()}/api/pads/${wildcardSlug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
      body: { content: "wildcard content", encrypted: false },
    })
      .its("status")
      .should("eq", 200);

    cy.request({
      method: "POST",
      url: `${apiUrl()}/api/pads/${outsideSlug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
      body: { content: "outside content", encrypted: false },
    })
      .its("status")
      .should("eq", 200);
  });

  it("creates a read-only role", () => {
    visitAuthed("/_/profile");
    cy.contains("button", "API").click();
    cy.get('input[placeholder="Role name"]').type("viewer");
    // Read is checked by default; Write/Delete stay unchecked.
    cy.contains("button", "Create role").click();
    cy.contains("viewer").should("be.visible");

    cy.request({
      url: `${apiUrl()}/roles`,
      headers: { Authorization: `Bearer ${token}` },
    }).then((res) => {
      const role = (res.body as { id: string; name: string }[]).find((r) => r.name === "viewer");
      expect(Boolean(role), "viewer role exists").to.eq(true);
      viewerRoleId = role!.id;
    });
  });

  it("creates a restricted key and attaches the viewer role", () => {
    visitAuthed("/_/profile");
    cy.contains("button", "API").click();
    cy.get('input[placeholder="Key name"]').type("scoped-bot");
    cy.contains("label", "Restricted").find('input[type="checkbox"]').check();
    cy.contains("button", "Create key").click();

    cy.get("code")
      .invoke("text")
      .then((key) => {
        scopedKey = key.trim();
      });
    cy.contains("button", "Dismiss").click();

    cy.contains('[data-cy="api-key-row"]', "scoped-bot").within(() => {
      cy.contains("button", "Roles").click();
      cy.contains("label", "viewer").find('input[type="checkbox"]').check();
    });
  });

  it("grants a wildcard ACL for the viewer role", () => {
    cy.request({
      method: "POST",
      url: `${apiUrl()}/acl`,
      headers: { Authorization: `Bearer ${token}` },
      body: { slug_pattern: `${wildcardBase}/*`, role_id: viewerRoleId },
    })
      .its("status")
      .should("eq", 201);
  });

  it("lets the scoped key read a pad under the wildcard prefix, but denies write and out-of-scope reads", () => {
    cy.request({
      url: `${apiUrl()}/api/pads/${wildcardSlug}`,
      headers: { Authorization: `Bearer ${scopedKey}` },
    }).then((res) => {
      expect(res.status).to.eq(200);
      expect(res.body.content).to.eq("wildcard content");
    });

    cy.request({
      method: "POST",
      url: `${apiUrl()}/api/pads/${wildcardSlug}`,
      headers: { Authorization: `Bearer ${scopedKey}` },
      body: { content: "attempted overwrite", encrypted: false },
      failOnStatusCode: false,
    })
      .its("status")
      .should("eq", 403);

    cy.request({
      url: `${apiUrl()}/api/pads/${outsideSlug}`,
      headers: { Authorization: `Bearer ${scopedKey}` },
      failOnStatusCode: false,
    })
      .its("status")
      .should("eq", 403);
  });

  it("revokes the full-access key and confirms it no longer authenticates", () => {
    visitAuthed("/_/profile");
    cy.contains("button", "API").click();
    cy.contains('[data-cy="api-key-row"]', "full-bot").within(() => {
      cy.contains("button", "Revoke").click();
    });
    cy.contains('[data-cy="api-key-row"]', "full-bot").should("contain.text", "revoked");

    cy.request({
      url: `${apiUrl()}/api/pads/${wildcardSlug}`,
      headers: { Authorization: `Bearer ${fullKey}` },
      failOnStatusCode: false,
    })
      .its("status")
      .should("eq", 401);
  });
});
