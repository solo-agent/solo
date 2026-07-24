export function orderCollaboratingIds(
  ids: string[],
  pairs: Array<[string, string]>,
): string[] {
  const known = new Set(ids);
  const neighbors = new Map<string, string[]>();
  for (const [from, to] of pairs) {
    if (!known.has(from) || !known.has(to)) continue;
    neighbors.set(from, [...(neighbors.get(from) ?? []), to]);
    neighbors.set(to, [...(neighbors.get(to) ?? []), from]);
  }

  const remaining = new Set(ids);
  const ordered: string[] = [];
  for (const id of ids) {
    if (!remaining.delete(id)) continue;
    const queue = [id];
    while (queue.length > 0) {
      const current = queue.shift()!;
      ordered.push(current);
      for (const neighbor of neighbors.get(current) ?? []) {
        if (remaining.delete(neighbor)) queue.push(neighbor);
      }
    }
  }
  return ordered;
}

export function reorderRankX(
  orderedIds: string[],
  positions: Map<string, { x: number; y: number }>,
): Map<string, number> {
  const order = new Map(orderedIds.map((id, index) => [id, index]));
  const ranks = new Map<number, string[]>();
  for (const id of orderedIds) {
    const position = positions.get(id);
    if (!position) continue;
    const rank = Math.round(position.y);
    ranks.set(rank, [...(ranks.get(rank) ?? []), id]);
  }

  const result = new Map<string, number>();
  for (const ids of ranks.values()) {
    const slots = ids.map((id) => positions.get(id)!.x).sort((a, b) => a - b);
    ids.sort((a, b) => order.get(a)! - order.get(b)!);
    ids.forEach((id, index) => result.set(id, slots[index]));
  }
  return result;
}
