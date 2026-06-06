// ============================================================================
// Button — single canonical button
// - Variants map directly to brutal design tokens (primary/danger/outline/ghost).
// - `default` and `destructive` retained as aliases for back-compat — existing
//   call sites still compile, but new code should use the semantic variants.
// - Sizes: default (h-10), sm (btn-brutal-sm h-8), lg (h-12), icon (h-10 w-10).
// ============================================================================

import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "btn-brutal inline-flex items-center justify-center whitespace-nowrap text-sm transition-colors focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        // Primary CTA (yellow call-to-action)
        primary: "btn-brutal-pink",
        // Dangerous action (destructive intent; white text on coral red)
        danger: "bg-brutal-danger text-white",
        // Outlined: white fill, 2px black border (already on the base)
        outline: "bg-brutal-white text-brutal-black",
        // Ghost: transparent until hover, then yellow tint
        ghost: "btn-flat",
        // ---- Deprecated aliases (kept for back-compat) ----
        default: "btn-brutal-pink",
        destructive: "bg-brutal-danger text-white",
        secondary: "bg-brutal-white text-brutal-black",
        link: "text-brutal-primary underline-offset-4 hover:underline",
      },
      size: {
        default: "h-10 px-3 py-2",
        sm: "btn-brutal-sm h-8 px-2.5 py-1.5 text-xs",
        lg: "h-12 px-4 py-2.5 text-base",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "default",
    },
  },
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    );
  },
);
Button.displayName = "Button";

export { Button, buttonVariants };
