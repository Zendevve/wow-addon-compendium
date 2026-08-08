// View registration contract (scaffold-owned). Every view module under
// `src/views/` exports `const view: View`; main.ts imports all of them.

export type ViewId =
  | "setup"
  | "overview"
  | "catalog"
  | "updates"
  | "collections"
  | "backups"
  | "savedvars"
  | "settings";

export interface View {
  id: string; // ?view=<id>
  label: string; // sidebar label
  icon: string; // icon id from icons.ts
  mount(host: HTMLElement): void | Promise<void>;
  unmount?(): void;
}
