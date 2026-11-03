// Storybook の story。Figma の variant 名と story 名を一致させる（claims/ui/UI-001.yaml 参照）。
export default { title: "PaymentList" };

export const Default = () => "<ul><li>支払一覧</li></ul>";
export const Empty = () => "<p>支払いはありません</p>";
export const Loading = () => "<p>読み込み中…</p>";
export const Error = () => "<p>読み込みに失敗しました</p>";
