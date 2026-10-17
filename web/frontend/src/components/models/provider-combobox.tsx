import { Input } from "@/components/ui/input"

interface ProviderComboboxProps {
  value: string
  onChange: (value: string) => void
  placeholder?: string
  suggestions: string[]
}

/** Free-form provider input with suggestions from models already configured. */
export function ProviderCombobox({
  value,
  onChange,
  placeholder,
  suggestions,
}: ProviderComboboxProps) {
  return (
    <>
      <Input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        list="model-provider-suggestions"
        autoComplete="off"
      />
      <datalist id="model-provider-suggestions">
        {suggestions.map((provider) => (
          <option key={provider} value={provider} />
        ))}
      </datalist>
    </>
  )
}
