/** Склеивает CSS-классы, пропуская пустые значения: cx('a', isB && 'b'). */
export function cx(...classes: Array<string | false | null | undefined>): string {
  return classes.filter(Boolean).join(' ')
}
