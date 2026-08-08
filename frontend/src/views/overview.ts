// Overview — the default landing view. Hosts the three health workflows
// (Scan / Doctor / Validation) behind a segmented control and delegates
// refresh() to whichever segment is active. Deep links (?view=scan …)
// still mount those views full-screen, bypassing this wrapper.

import type { AppState, Actions } from "../app";
import { mountScan } from "./scan";
import { mountDoctor } from "./doctor";
import { mountValidate } from "./validate";

type Segment = "scan" | "doctor" | "validate";

const SEGMENTS: { value: Segment; label: string }[] = [
  { value: "scan", label: "Scan" },
  { value: "doctor", label: "Doctor" },
  { value: "validate", label: "Validation" },
];

export function mountOverview(
  el: HTMLElement,
  app: AppState,
  actions: Actions,
): { refresh: () => void } {
  let active: Segment = "scan";
  let child: { refresh: () => void } | null = null;
  const childEl = document.createElement("div");
  childEl.className = "overview-child";

  const mountChild = (segment: Segment): void => {
    childEl.innerHTML = "";
    switch (segment) {
      case "scan":
        child = mountScan(childEl, app, actions);
        break;
      case "doctor":
        child = mountDoctor(childEl, app, actions);
        break;
      case "validate":
        child = mountValidate(childEl, app, actions);
        break;
    }
  };

  const segmented = document.createElement("div");
  segmented.className = "overview-segmented";
  segmented.innerHTML = SEGMENTS.map(
    (s) => `
      <button class="overview-seg" data-seg="${s.value}" aria-pressed="${s.value === active}">
        ${s.label}
      </button>`,
  ).join("");
  const segButtons = Array.from(
    segmented.querySelectorAll<HTMLButtonElement>("[data-seg]"),
  );

  const syncSegments = (): void => {
    segButtons.forEach((btn) => {
      const on = btn.dataset.seg === active;
      btn.classList.toggle("active", on);
      btn.setAttribute("aria-pressed", String(on));
    });
  };

  segButtons.forEach((btn, i) => {
    btn.addEventListener("click", () => {
      const segment = btn.dataset.seg as Segment;
      if (segment === active) return;
      active = segment;
      syncSegments();
      mountChild(active);
    });
    // Arrow keys move between segments, like the sidebar nav items.
    btn.addEventListener("keydown", (e) => {
      if (e.key !== "ArrowLeft" && e.key !== "ArrowRight") return;
      e.preventDefault();
      const next =
        (i + (e.key === "ArrowRight" ? 1 : segButtons.length - 1)) %
        segButtons.length;
      segButtons[next].focus();
      segButtons[next].click();
    });
  });

  const root = document.createElement("div");
  root.className = "overview";
  root.append(segmented, childEl);
  el.append(root);

  syncSegments();
  mountChild(active);

  return {
    refresh: () => {
      child?.refresh();
    },
  };
}
