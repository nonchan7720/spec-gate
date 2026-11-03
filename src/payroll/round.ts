/** 円未満切り捨て。マイナスは 0 に丸める。 docs/spec/rounding.md 参照 */
export function roundYen(amount: number): number {
  if (amount < 0) return 0;
  return Math.floor(amount);
}
