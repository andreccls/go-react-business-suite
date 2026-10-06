import { useId, type ReactNode } from 'react'

export interface ControlProps {
  id: string
  'aria-invalid': boolean
  'aria-describedby'?: string
  'aria-required'?: boolean
}

interface Props {
  label: string
  error?: string
  hint?: string
  required?: boolean
  children: (control: ControlProps) => ReactNode
}

/** Label + control + hint + error, wired together for screen readers. */
export function Field({ label, error, hint, required, children }: Props) {
  const id = useId()
  const hintId = `${id}-hint`
  const errId = `${id}-err`
  const described = [hint ? hintId : '', error ? errId : ''].filter(Boolean).join(' ') || undefined
  return (
    <div className="field">
      <label htmlFor={id}>
        {label}
        {required ? <span aria-hidden="true"> *</span> : null}
      </label>
      {children({ id, 'aria-invalid': Boolean(error), 'aria-describedby': described, 'aria-required': required })}
      {hint ? (
        <span id={hintId} className="hint">
          {hint}
        </span>
      ) : null}
      {error ? (
        <span id={errId} className="error">
          {error}
        </span>
      ) : null}
    </div>
  )
}
