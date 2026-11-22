// A story several outlets cover comes back from the API as one item per outlet, all with the same `story` id (the
// grouping step in internal/group). A list shows the event once: the item that ranks best stands for the group, the
// others are folded into it (its `coverage` says how many outlets there are).

type Grouped = { id: string; story?: string; importance?: number; time: string };

const better = (a: Grouped, b: Grouped) => (a.importance || 2) - (b.importance || 2) || Date.parse(a.time) - Date.parse(b.time);

// oneEach keeps, for every group, the item with the highest importance (an unscored one counts as 2, like score() in
// view.ts), the newest among equals, and drops the other members. Items with no group stay. The order of what is
// kept is the order of the input, so a list sorted by time stays sorted by time.
export function oneEach<T extends Grouped>(items: T[]): T[] {
  const best = new Map<string, T>();
  for (const it of items) {
    if (!it.story) continue;
    const cur = best.get(it.story);
    if (!cur || better(it, cur) > 0) best.set(it.story, it);
  }

  return items.filter((it) => !it.story || best.get(it.story) === it);
}

// sameStory reports whether two items are about the same event.
export const sameStory = (a: Grouped, b: Grouped) => Boolean(a.story) && a.story === b.story;
