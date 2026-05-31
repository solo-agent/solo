import * as React from "react";
import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/utils";

const buttonVariants = cva(
  "btn-brutal inline-flex items-center justify-center whitespace-nowrap text-sm transition-colors focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "btn-brutal-pink",
        destructive: "bg-brutal-red text-brutal-black",
        outline: "bg-brutal-white text-brutal-black",
        secondary: "bg-brutal-white text-brutal-black",
        ghost: "btn-flat",
        link: "text-brutal-pink underline-offset-4 hover:underline",
      },
      size: {
        default: "h-10 px-3 py-2",
        sm: "btn-brutal-sm h-8 px-2.5 py-1.5 text-xs",
        lg: "h-12 px-4 py-2.5 text-base",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "default",
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
