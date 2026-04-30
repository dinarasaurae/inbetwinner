import React from "react";
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export interface ButtonProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: "default" | "hero" | "heroSecondary" | "outline" | "ghost" | "link";
  size?: "default" | "sm" | "lg" | "icon";
  asChild?: boolean;
}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant = "default", size = "default", asChild = false, ...props }, ref) => {
    
    let variantClasses = "";
    switch (variant) {
      case "hero":
        variantClasses = "bg-primary text-primary-foreground hover:bg-primary/90";
        break;
      case "heroSecondary":
        variantClasses = "liquid-glass text-foreground hover:bg-white/5 border border-white/10";
        break;
      case "outline":
        variantClasses = "border border-input bg-transparent hover:bg-accent hover:text-accent-foreground";
        break;
      case "ghost":
        variantClasses = "hover:bg-accent hover:text-accent-foreground text-foreground";
        break;
      case "link":
        variantClasses = "text-primary underline-offset-4 hover:underline";
        break;
      default:
        variantClasses = "bg-primary text-primary-foreground hover:bg-primary/90";
    }

    let sizeClasses = "";
    switch (size) {
      case "sm":
        sizeClasses = "h-9 rounded-full px-4 text-xs";
        break;
      case "lg":
        sizeClasses = "h-11 rounded-full px-8";
        break;
      case "icon":
        sizeClasses = "h-10 w-10";
        break;
      default:
        sizeClasses = "h-10 px-6 py-2 rounded-full";
    }

    if(variant === "hero" || variant === "heroSecondary") {
      sizeClasses = size === "sm" ? "px-4 py-2 text-sm rounded-full" : "px-6 py-3 text-base font-medium rounded-full";
    }

    return (
      <button
        ref={ref}
        className={cn(
          "inline-flex items-center justify-center whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50",
          variantClasses,
          sizeClasses,
          className
        )}
        {...props}
      />
    );
  }
);
Button.displayName = "Button";

export { Button, cn };
