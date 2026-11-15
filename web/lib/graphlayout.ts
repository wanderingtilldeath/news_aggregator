import type { EntityNode, Graph } from "./api";

// Places entities on rings around a centre for the knowledge graph pictures: pure arithmetic, so the
// pictures are plain SVG made on the server with no script.

export type Placed = { name: string; count: number; x: number; y: number; r: number };

const radius = (count: number, max: number) => 7 + 15 * Math.sqrt(count / Math.max(max, 1));

// The overview: the busiest few in an inner ring, the rest on an outer ring.
export function overview(entities: EntityNode[], size: number, inner = 8, total = 36): Placed[] {
  const top = entities.slice(0, total);
  const max = top[0]?.count ?? 1;
  const c = size / 2;
  const ring = (nodes: EntityNode[], rad: number, offset: number) =>
    nodes.map((n, i): Placed => {
      const a = offset + (2 * Math.PI * i) / Math.max(nodes.length, 1);
      return { name: n.name, count: n.count, x: c + rad * Math.cos(a), y: c + rad * Math.sin(a), r: radius(n.count, max) };
    });

  return [...ring(top.slice(0, inner), size * 0.17, -Math.PI / 2), ...ring(top.slice(inner), size * 0.4, -Math.PI / 2 + Math.PI / Math.max(top.length - inner, 1))];
}

// One entity at the centre and the ones named with it around it.
export function ego(node: EntityNode, size: number): { centre: Placed; around: Placed[] } {
  const c = size / 2;
  const max = Math.max(node.count, ...node.related.map((r) => r.count), 1);
  const around = node.related.map((r, i): Placed => {
    const a = -Math.PI / 2 + (2 * Math.PI * i) / Math.max(node.related.length, 1);
    return { name: r.name, count: r.count, x: c + size * 0.36 * Math.cos(a), y: c + size * 0.36 * Math.sin(a), r: radius(r.count, max) };
  });

  return { centre: { name: node.name, count: node.count, x: c, y: c, r: radius(node.count, max) + 4 }, around };
}

export const linksOf = (g: Graph, placed: Placed[]) => {
  const at = new Map(placed.map((p) => [p.name, p]));
  return g.links.flatMap((l) => {
    const a = at.get(l.a);
    const b = at.get(l.b);
    return a && b ? [{ a, b, count: l.count }] : [];
  });
};

export const entityHref = (name: string) => `/entity/${encodeURIComponent(name)}`;
