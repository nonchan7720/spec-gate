// domain 層: 業務ロジックのみ。infra / app に依存してはならない。
export interface Invoice { id: string; amount: number }
export function total(items: Invoice[]): number {
  return items.reduce((s, i) => s + i.amount, 0);
}
