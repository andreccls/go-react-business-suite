import { useEffect, useId, useRef, type ReactNode } from 'react'
import { createPortal } from 'react-dom'

const FOCUSABLE = 'a[href],button:not([disabled]),input:not([disabled]),select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"])'

/** Accessible modal: labelled, Escape closes, Tab stays inside, focus goes in and comes back. */
export function Modal({ title, onClose, children }: { title: string; onClose: () => void; children: ReactNode }) {
  const titleId = useId()
  const box = useRef<HTMLDivElement>(null)
  const onCloseRef = useRef(onClose)
  useEffect(() => {
    onCloseRef.current = onClose
  })

  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null
    const el = box.current
    ;(el?.querySelector<HTMLElement>('input,select,textarea') ?? el?.querySelector<HTMLElement>(FOCUSABLE) ?? el)?.focus()

    function onKey(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        e.stopPropagation()
        onCloseRef.current()
      } else if (e.key === 'Tab' && el) {
        const items = [...el.querySelectorAll<HTMLElement>(FOCUSABLE)]
        const first = items[0]
        const last = items[items.length - 1]
        if (!first || !last) return
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault()
          last.focus()
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault()
          first.focus()
        }
      }
    }
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('keydown', onKey)
      previous?.focus()
    }
  }, [])

  return createPortal(
    // Pointer-only convenience: keyboard users close the dialog with Escape or the Cancel button.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div className="backdrop" onMouseDown={(e) => e.target === e.currentTarget && onClose()}>
      <div ref={box} className="modal" role="dialog" aria-modal="true" aria-labelledby={titleId} tabIndex={-1}>
        <h2 id={titleId}>{title}</h2>
        {children}
      </div>
    </div>,
    document.body,
  )
}
