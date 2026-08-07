// Hand-drawn inline icon set. Every icon shares one 24x24 viewBox and a
// 1.5px round-cap stroke so they read as one family at 16px and 20px.

export type IconName =
  | "shield"
  | "check"
  | "check-circle"
  | "alert"
  | "x-circle"
  | "info"
  | "search"
  | "refresh"
  | "wrench"
  | "trash"
  | "merge"
  | "flatten"
  | "edit"
  | "file"
  | "list"
  | "folder"
  | "package"
  | "upload"
  | "chevron-down"
  | "chevron-right"
  | "chevron-left"
  | "x"
  | "radar"
  | "table"
  | "warning-oct"
  | "archive";

const P: Record<IconName, string> = {
  shield:
    '<path d="M12 2.8 4.5 5.2v5.6c0 4.7 3.1 8.4 7.5 10.4 4.4-2 7.5-5.7 7.5-10.4V5.2L12 2.8Z"/><path d="m9.2 11.7 2 2 3.6-3.9"/>',
  check: '<path d="m5 12.5 4.5 4.5L19 7.5"/>',
  "check-circle":
    '<circle cx="12" cy="12" r="9"/><path d="m8.4 12.3 2.5 2.5 4.8-5.1"/>',
  alert:
    '<path d="M10.3 3.9 2.9 16.6c-.7 1.2.2 2.7 1.6 2.7h15c1.4 0 2.3-1.5 1.6-2.7L13.7 3.9c-.7-1.2-2.7-1.2-3.4 0Z"/><path d="M12 9.2v4.4"/><path d="M12 16.7v.1"/>',
  "x-circle":
    '<circle cx="12" cy="12" r="9"/><path d="m9 9 6 6"/><path d="m15 9-6 6"/>',
  info: '<circle cx="12" cy="12" r="9"/><path d="M12 11v5"/><path d="M12 7.8v.1"/>',
  search: '<circle cx="11" cy="11" r="6.5"/><path d="m20 20-4.1-4.1"/>',
  refresh:
    '<path d="M20 12a8 8 0 1 1-2.4-5.7"/><path d="M20 3.5V8h-4.5"/>',
  wrench:
    '<path d="M14.6 6.4a4.6 4.6 0 0 0-6 6L3.5 17.5 6.5 20.5 11.6 15.4a4.6 4.6 0 0 0 6-6L14.5 12.5 11.5 9.5l3.1-3.1Z"/><path d="m18.5 5.5 1.5-1.5"/>',
  trash:
    '<path d="M4.5 6.8h15"/><path d="M9 6.8V5a1.5 1.5 0 0 1 1.5-1.5h3A1.5 1.5 0 0 1 15 5v1.8"/><path d="M6.5 6.8 7.2 19a1.5 1.5 0 0 0 1.5 1.4h6.6a1.5 1.5 0 0 0 1.5-1.4l.7-12.2"/><path d="M10.3 10.5v6"/><path d="M13.7 10.5v6"/>',
  merge:
    '<path d="m12 3-7.5 4 7.5 4 7.5-4-7.5-4Z"/><path d="m4.5 11.5 7.5 4 7.5-4"/><path d="m4.5 15.5 7.5 4 7.5-4"/>',
  flatten:
    '<path d="M12 3.5v9"/><path d="m8.8 5.3 3.2-3.2 3.2 3.2"/><path d="M12 21v-4"/><path d="m8.8 19.3 3.2 3.2 3.2-3.2"/><path d="M4 12.5h16"/>',
  edit:
    '<path d="M4.5 19.5 6 15 16.8 4.2a1.9 1.9 0 0 1 2.7 0l.3.3a1.9 1.9 0 0 1 0 2.7L9 18l-4.5 1.5Z"/><path d="m14.8 6.2 3 3"/>',
  file:
    '<path d="M13.5 3.5H7a1.5 1.5 0 0 0-1.5 1.5v14A1.5 1.5 0 0 0 7 20.5h10a1.5 1.5 0 0 0 1.5-1.5V8l-5-4.5Z"/><path d="M13.5 3.5V8H19"/><path d="M9 12.5h6"/><path d="M9 16h4"/>',
  list: '<path d="M9 6.5h11"/><path d="M9 12h11"/><path d="M9 17.5h11"/><path d="M4.5 6.5h.1"/><path d="M4.5 12h.1"/><path d="M4.5 17.5h.1"/>',
  folder:
    '<path d="M3.5 6.5A1.5 1.5 0 0 1 5 5h4l2 2.5h8A1.5 1.5 0 0 1 20.5 9v8.5A1.5 1.5 0 0 1 19 19H5a1.5 1.5 0 0 1-1.5-1.5v-11Z"/>',
  package:
    '<path d="m12 2.8 8 3.7v11l-8 3.7-8-3.7v-11l8-3.7Z"/><path d="M4.3 6.6 12 10.2l7.7-3.6"/><path d="M12 10.2v9.9"/>',
  upload:
    '<path d="M12 15.5v-10"/><path d="m7.8 8.5 4.2-4.2 4.2 4.2"/><path d="M4.5 17.5v2A1.5 1.5 0 0 0 6 21h12a1.5 1.5 0 0 0 1.5-1.5v-2"/>',
  "chevron-down": '<path d="m6.5 9.5 5.5 5.5 5.5-5.5"/>',
  "chevron-right": '<path d="m9.5 6.5 5.5 5.5-5.5 5.5"/>',
  "chevron-left": '<path d="m14.5 6.5-5.5 5.5 5.5 5.5"/>',
  x: '<path d="m6 6 12 12"/><path d="M18 6 6 18"/>',
  radar:
    '<path d="M12 12 20 4"/><circle cx="12" cy="12" r="8.5"/><circle cx="12" cy="12" r="3.5"/>',
  table:
    '<rect x="3.5" y="4.5" width="17" height="15" rx="1.5"/><path d="M3.5 9.5h17"/><path d="M9.5 9.5v10"/><path d="M15 9.5v10"/>',
  "warning-oct":
    '<path d="M8.2 3.5h7.6L20.5 8.2v7.6l-4.7 4.7H8.2L3.5 15.8V8.2L8.2 3.5Z"/><path d="M12 8v4.5"/><path d="M12 16v.1"/>',
  archive:
    '<rect x="3.5" y="4" width="17" height="4" rx="1"/><path d="M5 8v11a1.5 1.5 0 0 0 1.5 1.5h11A1.5 1.5 0 0 0 19 19V8"/><path d="M10 12h4"/>',
};

const ATTRS =
  'xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" focusable="false"';

export function icon(name: IconName, size = 20, cls = ""): string {
  return `<svg class="${cls}" width="${size}" height="${size}" ${ATTRS}>${P[name]}</svg>`;
}
