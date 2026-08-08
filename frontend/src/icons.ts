// Inline icon set. Every icon shares one 24x24 viewBox and a 1.5px
// round-cap stroke so they read as one family at 16/18/20px.
// `icon(name, size?)` returns innerHTML-ready SVG (stroke=currentColor).

export type IconName =
  | "shield"
  | "refresh"
  | "search"
  | "stack"
  | "archive"
  | "save"
  | "edit"
  | "chevron-down"
  | "chevron-right"
  | "chevron-left"
  | "check"
  | "check-circle"
  | "alert"
  | "x"
  | "x-circle"
  | "plus"
  | "folder"
  | "trash"
  | "download"
  | "pin"
  | "info"
  | "external"
  | "settings"
  | "warning-oct";

const P: Record<IconName, string> = {
  shield:
    '<path d="M12 2.8 4.5 5.2v5.6c0 4.7 3.1 8.4 7.5 10.4 4.4-2 7.5-5.7 7.5-10.4V5.2L12 2.8Z"/><path d="m9.2 11.7 2 2 3.6-3.9"/>',
  refresh:
    '<path d="M20 12a8 8 0 1 1-2.4-5.7"/><path d="M20 3.5V8h-4.5"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="m20 20-4.1-4.1"/>',
  stack:
    '<rect x="4" y="3.5" width="16" height="4.5" rx="1"/><rect x="4" y="9.75" width="16" height="4.5" rx="1"/><rect x="4" y="16" width="16" height="4.5" rx="1"/>',
  archive:
    '<rect x="3.5" y="4" width="17" height="4" rx="1"/><path d="M5 8v11a1.5 1.5 0 0 0 1.5 1.5h11A1.5 1.5 0 0 0 19 19V8"/><path d="M10 12h4"/>',
  save:
    '<path d="M5 4.5h11.5L19.5 8v11a1.5 1.5 0 0 1-1.5 1.5H6A1.5 1.5 0 0 1 4.5 19V6A1.5 1.5 0 0 1 6 4.5Z"/><path d="M8 4.5V9h7.5V4.5"/><path d="M8 20.5V14h8v6.5"/>',
  edit:
    '<path d="M4.5 19.5 6 15 16.8 4.2a1.9 1.9 0 0 1 2.7 0l.3.3a1.9 1.9 0 0 1 0 2.7L9 18l-4.5 1.5Z"/><path d="m14.8 6.2 3 3"/>',
  "chevron-down": '<path d="m6.5 9.5 5.5 5.5 5.5-5.5"/>',
  "chevron-right": '<path d="m9.5 6.5 5.5 5.5-5.5 5.5"/>',
  "chevron-left": '<path d="m14.5 6.5-5.5 5.5 5.5 5.5"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  "check-circle":
    '<circle cx="12" cy="12" r="9"/><path d="m8.4 12.3 2.5 2.5 4.8-5.1"/>',
  alert:
    '<path d="M10.3 3.9 2.9 16.6c-.7 1.2.2 2.7 1.6 2.7h15c1.4 0 2.3-1.5 1.6-2.7L13.7 3.9c-.7-1.2-2.7-1.2-3.4 0Z"/><path d="M12 9.2v4.4"/><path d="M12 16.7v.1"/>',
  x: '<path d="m6 6 12 12"/><path d="M18 6 6 18"/>',
  "x-circle":
    '<circle cx="12" cy="12" r="9"/><path d="m9 9 6 6"/><path d="m15 9-6 6"/>',
  plus: '<path d="M12 5v14"/><path d="M5 12h14"/>',
  folder:
    '<path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4l2 2.5h8A1.5 1.5 0 0 1 20.5 9v8.5A1.5 1.5 0 0 1 19 19H5a1.5 1.5 0 0 1-1.5-1.5v-11Z"/>',
  trash:
    '<path d="M4.5 6.8h15"/><path d="M9 6.8V5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 15 5v1.8"/><path d="M6.5 6.8 7.2 19a1.5 1.5 0 0 0 1.5 1.4h6.6a1.5 1.5 0 0 0 1.5-1.4l.7-12.2"/><path d="M10.3 10.5v6"/><path d="M13.7 10.5v6"/>',
  download:
    '<path d="M12 3.5v10"/><path d="m7.8 9.3 4.2 4.2 4.2-4.2"/><path d="M4.5 17.5v2A1.5 1.5 0 0 0 6 21h12a1.5 1.5 0 0 0 1.5-1.5v-2"/>',
  pin: '<path d="M12 21s-7-5.4-7-11a7 7 0 0 1 14 0c0 5.6-7 11-7 11Z"/><circle cx="12" cy="10" r="2.6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.8v.1"/>',
  external:
    '<path d="M13.5 4.5H19.5V10.5"/><path d="m19.5 4.5-9 9"/><path d="M19.5 14v4a1.5 1.5 0 0 1-1.5 1.5H6A1.5 1.5 0 0 1 4.5 18V6A1.5 1.5 0 0 1 6 4.5h4"/>',
  settings:
    '<circle cx="12" cy="12" r="3.2"/><path d="M19.4 15a1.6 1.6 0 0 0 .3 1.8l.1.1a2 2 0 1 1-2.8 2.8l-.1-.1a1.6 1.6 0 0 0-1.8-.3 1.6 1.6 0 0 0-1 1.5V21a2 2 0 1 1-4 0v-.1a1.6 1.6 0 0 0-1-1.5 1.6 1.6 0 0 0-1.8.3l-.1.1a2 2 0 1 1-2.8-2.8l.1-.1a1.6 1.6 0 0 0 .3-1.8 1.6 1.6 0 0 0-1.5-1H3a2 2 0 1 1 0-4h.1a1.6 1.6 0 0 0 1.5-1 1.6 1.6 0 0 0-.3-1.8l-.1-.1a2 2 0 1 1 2.8-2.8l.1.1a1.6 1.6 0 0 0 1.8.3h0a1.6 1.6 0 0 0 1-1.5V3a2 2 0 1 1 4 0v.1a1.6 1.6 0 0 0 1 1.5h0a1.6 1.6 0 0 0 1.8-.3l.1-.1a2 2 0 1 1 2.8 2.8l-.1.1a1.6 1.6 0 0 0-.3 1.8v0a1.6 1.6 0 0 0 1.5 1H21a2 2 0 1 1 0 4h-.1a1.6 1.6 0 0 0-1.5 1Z"/>',
  "warning-oct":
    '<path d="M8.2 3.5h7.6L20.5 8.2v7.6l-4.7 4.7H8.2L3.5 15.8V8.2L8.2 3.5Z"/><path d="M12 8v4.5"/><path d="M12 16v.1"/>',
};

const ATTRS =
  'xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"';

export function icon(name: IconName, size = 20): string {
  return `<svg width="${size}" height="${size}" ${ATTRS}>${P[name]}</svg>`;
}
