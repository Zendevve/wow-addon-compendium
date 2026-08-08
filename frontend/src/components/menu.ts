// Row overflow menu: a compact ⋯ trigger that opens a small popover of real
// buttons. Keyboard contract: opening moves focus to the first item,
// ArrowUp/Down (plus Home/End) move between items, Esc closes and returns
// focus to the trigger, Tab closes and advances focus, outside clicks close.
// Styling lives in components/menu.css; the trigger is a token-styled button
// and the popover reuses the component primitives (tokens, focus ring).

import type { IconName } from "../icons";
import { icon } from "../icons";

export interface MenuItem {
  label: string;
  icon?: IconName;
  /** Amber treatment for restore-type actions (matches .btn-restore). */
  warn?: boolean;
  disabled?: boolean;
  onSelect: () => void;
}

export interface RowMenuOptions {
  /** aria-label on the trigger, e.g. "More actions for ElvUI". */
  label: string;
  /** aria-label on the popover, e.g. "Actions for ElvUI". */
  menuLabel: string;
  /** Items are built on every open so labels reflect current state. */
  items: () => MenuItem[];
}

let menuSeq = 0;

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), a[href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

export function attachRowMenu(trigger: HTMLElement, opts: RowMenuOptions): void {
  // Wrap the trigger so the popover can be positioned against it; the
  // wrapper also lets .closest() find the row's menu from inside the popover.
  const wrap = document.createElement("span");
  wrap.className = "row-menu";
  if (trigger.dataset.rowMenu) wrap.dataset.rowMenu = trigger.dataset.rowMenu;
  trigger.parentNode?.insertBefore(wrap, trigger);
  wrap.appendChild(trigger);

  trigger.setAttribute("aria-haspopup", "menu");
  trigger.setAttribute("aria-expanded", "false");
  trigger.setAttribute("aria-label", opts.label);

  let pop: HTMLDivElement | null = null;

  const enabledItems = (): HTMLButtonElement[] =>
    pop
      ? Array.from(pop.querySelectorAll<HTMLButtonElement>(".menu-item:not([disabled])"))
      : [];

  const focusAt = (list: HTMLButtonElement[], i: number): void => {
    if (list.length === 0) return;
    const idx = ((i % list.length) + list.length) % list.length;
    list[idx].focus();
  };

  const close = (returnFocus: boolean): void => {
    if (!pop) return;
    pop.remove();
    pop = null;
    trigger.setAttribute("aria-expanded", "false");
    trigger.removeAttribute("aria-controls");
    if (returnFocus) trigger.focus();
  };

  const focusSibling = (backward: boolean): void => {
    const focusables = Array.from(document.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR));
    const idx = focusables.indexOf(trigger);
    if (idx === -1) return;
    const step = backward ? -1 : 1;
    for (let i = idx + step; i >= 0 && i < focusables.length; i += step) {
      if (focusables[i].offsetParent === null) continue; // hidden (display:none)
      focusables[i].focus();
      return;
    }
  };

  const open = (): void => {
    if (pop) return;
    const built = opts.items();
    const id = `row-menu-${++menuSeq}`;
    pop = document.createElement("div");
    pop.className = "menu-pop";
    pop.setAttribute("role", "menu");
    pop.setAttribute("aria-label", opts.menuLabel);
    pop.id = id;
    pop.innerHTML = built
      .map(
        (it) =>
          `<button class="menu-item${it.warn ? " menu-item-warn" : ""}" role="menuitem" ${it.disabled ? "disabled" : ""} tabindex="-1">${it.icon ? icon(it.icon, 15) : ""}<span>${escapeHtml(it.label)}</span></button>`,
      )
      .join("");
    wrap.appendChild(pop);
    trigger.setAttribute("aria-expanded", "true");
    trigger.setAttribute("aria-controls", id);
    pop.querySelectorAll<HTMLButtonElement>(".menu-item").forEach((btn, i) => {
      btn.addEventListener("click", () => {
        close(false);
        built[i].onSelect();
      });
    });
    focusAt(enabledItems(), 0);
  };

  trigger.addEventListener("click", (e) => {
    e.stopPropagation();
    if (pop) close(true);
    else open();
  });

  wrap.addEventListener("keydown", (e) => {
    if (!pop) return;
    const list = enabledItems();
    const current = list.indexOf(document.activeElement as HTMLButtonElement);
    switch (e.key) {
      case "Escape":
        e.preventDefault();
        e.stopPropagation();
        close(true);
        break;
      case "ArrowDown":
        e.preventDefault();
        focusAt(list, current === -1 ? 0 : current + 1);
        break;
      case "ArrowUp":
        e.preventDefault();
        focusAt(list, current === -1 ? list.length - 1 : current - 1);
        break;
      case "Home":
        e.preventDefault();
        focusAt(list, 0);
        break;
      case "End":
        e.preventDefault();
        focusAt(list, list.length - 1);
        break;
      case "Tab":
        e.preventDefault();
        e.stopPropagation();
        close(false);
        focusSibling(e.shiftKey);
        break;
    }
  });

  document.addEventListener("pointerdown", (e) => {
    if (pop && e.target instanceof Node && !wrap.contains(e.target)) close(false);
  });
}

const ESC_HTML: Record<string, string> = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
};
function escapeHtml(s: string): string {
  return s.replace(/[&<>"]/g, (c) => ESC_HTML[c]);
}
