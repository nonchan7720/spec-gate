// app 層: domain と infra の両方に依存してよい。
import { total } from "../domain/invoice";
import { findAll } from "../infra/invoiceRepo";
export async function handle() {
  return total(await findAll());
}
