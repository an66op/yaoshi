export function keepMenuItemVisible(container: HTMLElement, item: HTMLElement, padding = 10) {
  const containerRect = container.getBoundingClientRect()
  const itemRect = item.getBoundingClientRect()
  const visibleTop = containerRect.top + padding
  const visibleBottom = containerRect.bottom - padding
  if (itemRect.top < visibleTop) container.scrollTop -= visibleTop - itemRect.top
  else if (itemRect.bottom > visibleBottom) container.scrollTop += itemRect.bottom - visibleBottom
}
