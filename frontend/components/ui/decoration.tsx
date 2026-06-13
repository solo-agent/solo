// ============================================================================
// Decoration — v3.3: a small "sticker" primitive for ornamenting hero
// surfaces, empty states, and card corners.
//
// Composes a 2px black border + colored fill + hard shadow + a lucide
// icon, then optionally spins/bounces/pulses. Designed to be absolutely
// positioned by the caller (`absolute -top-3 -right-3` etc.) so the
// surrounding layout is unaffected.
//
// Usage:
//   <div className="relative">
//     <Decoration shape="star" color="accent" animation="spin" />
//   </div>
// ============================================================================

import * as React from "react";
import { Star, Sparkles, Zap, ArrowUpRight, Heart, Flame } from "lucide-react";
import { cn } from "@/lib/utils";

export type DecorationShape =
  | "star"
  | "sparkle"
  | "zap"
  | "arrow"
  | "heart"
  | "flame";

export type DecorationColor =
  | "primary"
  | "accent"
  | "info"
  | "success"
  | "warning"
  | "violet"
  | "danger";

export type DecorationSize = "sm" | "md" | "lg";

export type DecorationAnimation = "none" | "spin" | "bounce" | "pulse";

const SHAPE_ICON: Record<DecorationShape, React.ComponentType<{ className?: string }>> = {
  star: Star,
  sparkle: Sparkles,
  zap: Zap,
  arrow: ArrowUpRight,
  heart: Heart,
  flame: Flame,
};

const SIZE_CLASS: Record<DecorationSize, { box: string; icon: string }> = {
  sm: { box: "h-8 w-8", icon: "h-4 w-4" },
  md: { box: "h-12 w-12", icon: "h-6 w-6" },
  lg: { box: "h-16 w-16", icon: "h-8 w-8" },
};

const BG_CLASS: Record<DecorationColor, string> = {
  primary: "bg-brutal-primary",
  accent: "bg-brutal-accent",
  info: "bg-brutal-info",
  success: "bg-brutal-success",
  warning: "bg-brutal-warning",
  violet: "bg-brutal-violet",
  danger: "bg-brutal-danger",
};

const ANIM_CLASS: Record<DecorationAnimation, string> = {
  none: "",
  spin: "animate-spin-slow",
  bounce: "animate-bounce-slow",
  pulse: "animate-pulse-brutal",
};

export interface DecorationProps {
  shape?: DecorationShape;
  color?: DecorationColor;
  size?: DecorationSize;
  animation?: DecorationAnimation;
  /** Sticker rotation in degrees. */
  rotation?: number;
  className?: string;
}

export function Decoration({
  shape = "star",
  color = "accent",
  size = "md",
  animation = "none",
  rotation = 0,
  className,
}: DecorationProps) {
  const Icon = SHAPE_ICON[shape];
  const { box, icon } = SIZE_CLASS[size];

  return (
    <div
      style={rotation !== 0 ? { transform: `rotate(${rotation}deg)` } : undefined}
      className={cn(
        "flex items-center justify-center border-2 border-black shadow-brutal-sm",
        box,
        BG_CLASS[color],
        ANIM_CLASS[animation],
        className,
      )}
    >
      <Icon className={cn(icon, "text-black")} />
    </div>
  );
}
