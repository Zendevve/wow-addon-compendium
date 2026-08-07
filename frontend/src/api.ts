// Typed facade over the Wails binding `window.go.service.Service`.
// In mock mode (or any browser without a Go backend) the mock service is
// installed at module load, so every call below follows one code path.

import type { Service } from "./types";
import { installMockIfNeeded } from "./mock";

export const mockActive = installMockIfNeeded();

type RawService = Record<string, (...args: unknown[]) => Promise<unknown>>;

function rawService(): RawService {
  const g = window as unknown as {
    go?: { service?: { Service?: RawService } };
  };
  const s = g.go?.service?.Service;
  if (!s) throw new Error("wowfix Service binding is unavailable");
  return s;
}

function call<K extends keyof Service>(
  name: K,
  ...args: Parameters<Service[K]>
): ReturnType<Service[K]> {
  const fn = rawService()[name];
  return normalize(name, fn(...args)) as ReturnType<Service[K]>;
}

// Go marshals nil slices as JSON `null`, so every slice field is coerced to
// [] before the views touch it — `a.issues.map(...)` must never throw.
function normalize(name: keyof Service, p: Promise<unknown>): Promise<unknown> {
  return p.then((raw) => {
    if (raw === null || raw === undefined) return raw;
    switch (name) {
      case "Scan": {
        const s = raw as Record<string, unknown>;
        const addons = (s.addons ?? []) as Record<string, unknown>[];
        for (const a of addons) {
          const issues = (a.issues ?? []) as Record<string, unknown>[];
          for (const i of issues) i.options = i.options ?? [];
          a.issues = issues;
          a.compat = a.compat ?? [];
        }
        s.addons = addons;
        s.errors = s.errors ?? [];
        return s;
      }
      case "DetectInstalls":
      case "Profiles":
        return Array.isArray(raw) ? raw : [];
      case "Validate": {
        const v = raw as Record<string, unknown>;
        v.addons = v.addons ?? [];
        return v;
      }
      case "Fix":
      case "FixAll": {
        const f = raw as Record<string, unknown>;
        f.fixes = f.fixes ?? [];
        return f;
      }
      case "InstallZip": {
        const z = raw as Record<string, unknown>;
        z.errors = z.errors ?? [];
        return z;
      }
      case "CheckUpdates": {
        const u = raw as Record<string, unknown>;
        u.updates = u.updates ?? [];
        u.errors = u.errors ?? [];
        return u;
      }
      case "ApplyUpdate":
      case "ApplyAllUpdates": {
        const b = raw as Record<string, unknown>;
        b.applied = b.applied ?? [];
        return b;
      }
      case "SearchCatalog": {
        const c = raw as Record<string, unknown>;
        c.results = c.results ?? [];
        c.errors = c.errors ?? [];
        return c;
      }
      case "InstallSource": {
        const s = raw as Record<string, unknown>;
        s.installed = s.installed ?? [];
        s.replaced = s.replaced ?? [];
        s.skipped = s.skipped ?? [];
        s.errors = s.errors ?? [];
        return s;
      }
      default:
        return raw;
    }
  });
}

export const service: Service = {
  GetState: () => call("GetState"),
  DetectInstalls: () => call("DetectInstalls"),
  SetInstall: (root, flavor) => call("SetInstall", root, flavor),
  SetProfile: (id) => call("SetProfile", id),
  Profiles: () => call("Profiles"),
  Scan: () => call("Scan"),
  Validate: () => call("Validate"),
  Fix: (folderName, allowDestructive) =>
    call("Fix", folderName, allowDestructive),
  FixAll: (allowDestructive) => call("FixAll", allowDestructive),
  InstallZip: (zipPath, allowReplace) =>
    call("InstallZip", zipPath, allowReplace),
  CheckUpdates: () => call("CheckUpdates"),
  ApplyUpdate: (folder, allowReplace) =>
    call("ApplyUpdate", folder, allowReplace),
  ApplyAllUpdates: (allowReplace) => call("ApplyAllUpdates", allowReplace),
  SearchCatalog: (query) => call("SearchCatalog", query),
  InstallSource: (source, allowReplace) =>
    call("InstallSource", source, allowReplace),
};
