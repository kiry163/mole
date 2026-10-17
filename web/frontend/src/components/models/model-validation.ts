/**
 * Validate a model identifier. The identifier is passed to the upstream
 * endpoint verbatim; only obviously malformed values are rejected.
 */
export type ValidationLevel = "error" | "warning" | "success"

export interface FieldValidation {
  level: ValidationLevel
  messageKey: string
  messageParams?: Record<string, string>
  fix?: string
}

export function validateModelField(input: string): FieldValidation {
  const trimmed = input.trim()
  if (!trimmed) return { level: "success", messageKey: "" }

  if (/\s/.test(trimmed)) {
    return {
      level: "error",
      messageKey: "models.validation.whitespace",
      fix: trimmed.replace(/\s+/g, ""),
    }
  }
  if (trimmed.startsWith("/")) {
    return {
      level: "error",
      messageKey: "models.validation.leadingSlash",
      fix: trimmed.replace(/^\/+/, ""),
    }
  }
  if (trimmed.includes("//")) {
    return {
      level: "error",
      messageKey: "models.validation.consecutiveSlash",
      fix: trimmed.replace(/\/+/g, "/"),
    }
  }
  return { level: "success", messageKey: "" }
}
