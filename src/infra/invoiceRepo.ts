// infra 層: domain に依存してよい（図のとおり）。
import type { Invoice } from "../domain/invoice";
export async function findAll(): Promise<Invoice[]> {
  return [];
}
