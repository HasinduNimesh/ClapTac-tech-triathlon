import test from "node:test";
import assert from "node:assert/strict";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { fileURLToPath } from "node:url";
import ts from "typescript";

const sourceRoot = new URL("../src/", import.meta.url);
const rootPath = fileURLToPath(sourceRoot);
const files = [];
function visit(directory) {
  for (const name of readdirSync(directory)) {
    const path = join(directory, name);
    if (statSync(path).isDirectory()) visit(path);
    else if (path.endsWith(".tsx")) files.push(path);
  }
}
visit(rootPath);

const sourceFiles = files.map((path) => ({
  path,
  source: ts.createSourceFile(path, readFileSync(path, "utf8"), ts.ScriptTarget.Latest, true, ts.ScriptKind.TSX),
}));

function jsxTagName(node) {
  if (ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) {
    return ts.isIdentifier(node.tagName) ? node.tagName.text.toLowerCase() : "";
  }
  return "";
}

function hasAttribute(node, name) {
  return node.attributes.properties.some((attribute) =>
    ts.isJsxAttribute(attribute) && ts.isIdentifier(attribute.name) && attribute.name.text === name,
  );
}

function attributeString(node, name) {
  const attribute = node.attributes.properties.find((item) =>
    ts.isJsxAttribute(item) && ts.isIdentifier(item.name) && item.name.text === name,
  );
  return attribute && ts.isJsxAttribute(attribute) && attribute.initializer && ts.isStringLiteral(attribute.initializer)
    ? attribute.initializer.text
    : undefined;
}

const explicitLabelIds = new Set();
for (const { source } of sourceFiles) {
  function collect(node) {
    if ((ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node)) && jsxTagName(node) === "label") {
      const target = attributeString(node, "htmlFor");
      if (target) explicitLabelIds.add(target);
    }
    ts.forEachChild(node, collect);
  }
  collect(source);
}

test("every form control and button in every role screen has an accessible name", () => {
  const problems = [];

  for (const { path, source } of sourceFiles) {
    function inspect(node, labelAncestor = false) {
      const tag = jsxTagName(node);
      const isOpening = ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node);
      const nowInsideLabel = labelAncestor || (isOpening && tag === "label") ||
        (ts.isJsxElement(node) && jsxTagName(node.openingElement) === "label");

      if (isOpening && ["input", "select", "textarea"].includes(tag)) {
        const type = attributeString(node, "type")?.toLowerCase();
        const id = attributeString(node, "id");
        const named = hasAttribute(node, "aria-label") || hasAttribute(node, "aria-labelledby") ||
          nowInsideLabel || (id && explicitLabelIds.has(id));
        if (type !== "hidden" && !named) problems.push(`${path}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1} <${tag}> has no label or ARIA name`);
      }

      if (isOpening && tag === "button") {
        const namedByAttribute = hasAttribute(node, "aria-label") || hasAttribute(node, "aria-labelledby") || hasAttribute(node, "title");
        const hasChildren = ts.isJsxOpeningElement(node) && node.parent && ts.isJsxElement(node.parent) &&
          node.parent.children.some((child) => !ts.isJsxText(child) || child.getText(source).trim().length > 0);
        if (!namedByAttribute && !hasChildren) problems.push(`${path}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1} <button> has no accessible name`);
      }

      ts.forEachChild(node, (child) => inspect(child, nowInsideLabel));
    }
    inspect(source);
  }

  assert.deepEqual(problems, [], problems.join("\n"));
});

test("every anchor and router link has link text or an explicit accessible name", () => {
  const problems = [];

  for (const { path, source } of sourceFiles) {
    function inspect(node) {
      const tag = jsxTagName(node);
      const isOpening = ts.isJsxOpeningElement(node) || ts.isJsxSelfClosingElement(node);
      if (isOpening && ["a", "link"].includes(tag)) {
        const namedByAttribute = hasAttribute(node, "aria-label") || hasAttribute(node, "aria-labelledby") || hasAttribute(node, "title");
        const hasReadableChild = ts.isJsxOpeningElement(node) && node.parent && ts.isJsxElement(node.parent) &&
          node.parent.children.some((child) => {
            if (ts.isJsxText(child)) return child.getText(source).trim().length > 0;
            if (ts.isJsxExpression(child)) return !!child.expression;
            return ts.isJsxElement(child) || ts.isJsxSelfClosingElement(child);
          });
        if (!namedByAttribute && !hasReadableChild) {
          problems.push(`${path}:${source.getLineAndCharacterOfPosition(node.getStart(source)).line + 1} <${tag}> has no link text or ARIA name`);
        }
      }
      ts.forEachChild(node, inspect);
    }
    inspect(source);
  }

  assert.deepEqual(problems, [], problems.join("\n"));
});

test("OIDC sign-in startup is announced through a polite live status region", () => {
  const loginPage = readFileSync(join(rootPath, "routes", "LoginPage.tsx"), "utf8");
  assert.match(loginPage, /<p role="status" aria-live="polite">\{t\("Connecting to identity provider…"\)\}<\/p>/);
  assert.match(loginPage, /<p role="alert">\{t\("Sign-in could not be started\. Check your connection and try again\."\)\}<\/p>/);
});
