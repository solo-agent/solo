// ============================================================================
// Select — brutal-styled native <select>
// - Wraps the native element with brutal borders + hard shadow so we don't
//   ship a Radix dependency just for styling.
// - Native <select> keeps full keyboard + a11y + mobile picker support.
// - Use this for simple value-pickers (agent selector, status filter, etc.).
//   For complex multi-selects or search, build a Combobox on top later.
// ============================================================================

import * as React from "react";
import { ChevronDown } from "lucide-react";
import { cn } from "@/lib/utils";

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean;
}

export interface SelectProps
  extends Omit<React.SelectHTMLAttributes<HTMLSelectElement>, "size"> {
  options: SelectOption[];
  /** Optional visible label rendered above the select. */
  label?: string;
  /** Placeholder shown when value is empty (renders a disabled option). */
  placeholder?: string;
  /** Visual size; default matches btn-brutal-sm proportions. */
  size?: "sm" | "md";
  className?: string;
}

const SIZE_CLASSES: Record<NonNullable<SelectProps["size"]>, string> = {
  sm: "h-8 px-2 text-xs",
  md: "h-10 px-3 text-sm",
};

export const Select = React.forwardRef<HTMLSelectElement, SelectProps>(
  (
    {
      options,
      label,
      placeholder,
      size = "sm",
      className,
      value,
      onChange,
      ...props
    },
    ref,
  ) => {
    const selectEl = (
      <div className="relative inline-block">
        <select
          ref={ref}
          value={value}
          onChange={onChange}
          className={cn(
            "appearance-none pr-7",
            "bg-white text-black font-heading font-bold",
            "border-2 border-black shadow-brutal-sm",
            "focus-visible:outline-none focus-visible:shadow-brutal",
            "disabled:opacity-50 disabled:pointer-events-none",
            SIZE_CLASSES[size],
            className,
          )}
          {...props}
        >
          {placeholder && (
            <option value="" disabled hidden>
              {placeholder}
            </option>
          )}
          {options.map((opt) => (
            <option key={opt.value} value={opt.value} disabled={opt.disabled}>
              {opt.label}
            </option>
          ))}
        </select>
        <ChevronDown
          aria-hidden
          className="pointer-events-none absolute right-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-black"
        />
      </div>
    );

    if (label) {
      return (
        <label className="flex flex-col gap-1">
          <span className="font-heading text-[10px] font-bold uppercase tracking-widest text-foreground">
            {label}
          </span>
          {selectEl}
        </label>
      );
    }
    return selectEl;
  },
);
Select.displayName = "Select";
