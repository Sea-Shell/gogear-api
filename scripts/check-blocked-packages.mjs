import fs from "node:fs";
import path from "node:path";

const blockedPackages = {
  axios: ["1.14.1", "0.30.4"],
  "plain-crypto-js": ["4.2.1"],
};

const blockedNamespaces = ["@emilgroup/"];

const manifestFiles = ["package.json", "package-lock.json"];
const violations = [];

for (const relativePath of manifestFiles) {
  const filePath = path.resolve(process.cwd(), relativePath);

  if (!fs.existsSync(filePath)) {
    continue;
  }

  const content = fs.readFileSync(filePath, "utf8");
  const json = JSON.parse(content);

  checkManifestSpecs(relativePath, json);
  checkLockfilePackages(relativePath, json);
}

if (violations.length > 0) {
  console.error("Blocked packages or namespaces detected:");
  for (const violation of violations) {
    console.error(`- ${violation}`);
  }
  process.exit(1);
}

console.log("No blocked package versions found in package manifests or lockfiles.");

function checkManifestSpecs(relativePath, json) {
  const dependencySections = [
    "dependencies",
    "devDependencies",
    "optionalDependencies",
    "peerDependencies",
    "bundleDependencies",
    "bundledDependencies",
    "overrides",
    "resolutions",
  ];

  for (const section of dependencySections) {
    const dependencies = json?.[section];
    if (!dependencies || typeof dependencies !== "object") {
      continue;
    }

    for (const [name, spec] of Object.entries(dependencies)) {
      if (isBlockedNamespace(name)) {
        violations.push(`${relativePath} ${section}.${name}=${spec} belongs to blocked namespace ${getBlockedNamespace(name)}`);
      }

      if (!blockedPackages[name] || typeof spec !== "string") {
        continue;
      }

      for (const blockedVersion of blockedPackages[name]) {
        if (rangeCouldResolveTo(spec, blockedVersion)) {
          violations.push(`${relativePath} ${section}.${name}=${spec} can resolve to blocked version ${blockedVersion}`);
        }
      }
    }
  }

  const rootLockDependencies = json?.packages?.[""];
  if (!rootLockDependencies || typeof rootLockDependencies !== "object") {
    return;
  }

  for (const section of ["dependencies", "devDependencies", "optionalDependencies", "peerDependencies"]) {
    const dependencies = rootLockDependencies[section];
    if (!dependencies || typeof dependencies !== "object") {
      continue;
    }

    for (const [name, spec] of Object.entries(dependencies)) {
      if (isBlockedNamespace(name)) {
        violations.push(`${relativePath} packages[""].${section}.${name}=${spec} belongs to blocked namespace ${getBlockedNamespace(name)}`);
      }

      if (!blockedPackages[name] || typeof spec !== "string") {
        continue;
      }

      for (const blockedVersion of blockedPackages[name]) {
        if (rangeCouldResolveTo(spec, blockedVersion)) {
          violations.push(`${relativePath} packages[""].${section}.${name}=${spec} can resolve to blocked version ${blockedVersion}`);
        }
      }
    }
  }
}

function checkLockfilePackages(relativePath, json) {
  const lockPackages = json?.packages;
  if (!lockPackages || typeof lockPackages !== "object") {
    return;
  }

  for (const [packagePath, pkg] of Object.entries(lockPackages)) {
    if (!pkg || typeof pkg !== "object") {
      continue;
    }

    const name = packagePath.startsWith("node_modules/") ? packagePath.slice("node_modules/".length) : pkg.name;
    const version = pkg.version;

    if (name && isBlockedNamespace(name)) {
      violations.push(`${relativePath} ${packagePath || "packages[\"\"]"} installs package ${name} from blocked namespace ${getBlockedNamespace(name)}`);
    }

    if (!name || typeof version !== "string" || !blockedPackages[name]?.includes(version)) {
      continue;
    }

    violations.push(`${relativePath} ${packagePath || "packages[\"\"]"} installs blocked version ${name}@${version}`);
  }
}

function rangeCouldResolveTo(spec, blockedVersion) {
  const normalizedSpec = spec.trim();
  if (!normalizedSpec) {
    return false;
  }

  return normalizedSpec
    .split("||")
    .map((part) => part.trim())
    .some((part) => comparatorSetAllowsVersion(part, blockedVersion));
}

function isBlockedNamespace(name) {
  return Boolean(getBlockedNamespace(name));
}

function getBlockedNamespace(name) {
  return blockedNamespaces.find((namespace) => name.startsWith(namespace));
}

function comparatorSetAllowsVersion(spec, version) {
  if (["*", "latest"].includes(spec)) {
    return true;
  }

  if (/^[~^]?\d+\.\d+\.\d+$/.test(spec)) {
    return matchesSimpleRange(spec, version);
  }

  const comparators = spec.split(/\s+/).filter(Boolean);
  if (comparators.length === 0) {
    return false;
  }

  return comparators.every((comparator) => matchesComparator(comparator, version));
}

function matchesSimpleRange(spec, version) {
  const operator = ["^", "~"].includes(spec[0]) ? spec[0] : "";
  const baseVersion = operator ? spec.slice(1) : spec;

  if (operator === "") {
    return compareVersions(baseVersion, version) === 0;
  }

  const lowerBound = parseVersion(baseVersion);
  const upperBound = operator === "^" ? getCaretUpperBound(lowerBound) : getTildeUpperBound(lowerBound);

  return compareVersions(version, lowerBound) >= 0 && compareVersions(version, upperBound) < 0;
}

function matchesComparator(comparator, version) {
  const match = comparator.match(/^(<=|>=|<|>|=)?(\d+\.\d+\.\d+)$/);
  if (!match) {
    return false;
  }

  const [, operator = "=", targetVersion] = match;
  const comparison = compareVersions(version, targetVersion);

  switch (operator) {
    case "<":
      return comparison < 0;
    case "<=":
      return comparison <= 0;
    case ">":
      return comparison > 0;
    case ">=":
      return comparison >= 0;
    case "=":
      return comparison === 0;
    default:
      return false;
  }
}

function getCaretUpperBound(version) {
  const [major, minor, patch] = parseVersion(version);

  if (major > 0) {
    return [major + 1, 0, 0];
  }

  if (minor > 0) {
    return [0, minor + 1, 0];
  }

  return [0, 0, patch + 1];
}

function getTildeUpperBound(version) {
  const [major, minor] = parseVersion(version);
  return [major, minor + 1, 0];
}

function compareVersions(left, right) {
  const leftParts = parseVersion(left);
  const rightParts = parseVersion(right);

  for (let index = 0; index < 3; index += 1) {
    if (leftParts[index] !== rightParts[index]) {
      return leftParts[index] - rightParts[index];
    }
  }

  return 0;
}

function normalizeVersion(version) {
  return parseVersion(version).join(".");
}

function parseVersion(version) {
  if (Array.isArray(version)) {
    return version;
  }

  return `${version}`.split(".").map((part) => Number.parseInt(part, 10));
}