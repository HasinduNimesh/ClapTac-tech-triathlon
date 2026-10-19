import { useEffect, useRef, type CSSProperties, type ReactNode } from "react";
import { useLocale } from "../i18n";

/* A small isometric city built from CSS 3D boxes. Everything here is decorative: the vehicles
   follow CSS motion paths on the road ring, and the pointer tilts the whole stage a few degrees. */

const SIZE = 440;
const CX1 = 50;
const CY1 = 90;
const CX2 = 390;
const CY2 = 350;

/** A rounded-rectangle ring road path, inset from the road centre line. */
function ring(inset: number, clockwise: boolean) {
  const x1 = CX1 + inset;
  const y1 = CY1 + inset;
  const x2 = CX2 - inset;
  const y2 = CY2 - inset;
  const r = Math.max(6, 24 - inset);
  return clockwise
    ? `M${x1 + r} ${y1} H${x2 - r} Q${x2} ${y1} ${x2} ${y1 + r} V${y2 - r} Q${x2} ${y2} ${x2 - r} ${y2} H${x1 + r} Q${x1} ${y2} ${x1} ${y2 - r} V${y1 + r} Q${x1} ${y1} ${x1 + r} ${y1} Z`
    : `M${x1 + r} ${y1} Q${x1} ${y1} ${x1} ${y1 + r} V${y2 - r} Q${x1} ${y2} ${x1 + r} ${y2} H${x2 - r} Q${x2} ${y2} ${x2} ${y2 - r} V${y1 + r} Q${x2} ${y1} ${x2 - r} ${y1} Z`;
}

type BoxProps = { x: number; y: number; w: number; d: number; h: number; tone: string; children?: ReactNode };

function Box({ x, y, w, d, h, tone, children }: BoxProps) {
  const style = { left: x, top: y, width: w, height: d, "--h": `${h}px` } as CSSProperties;
  return (
    <div className={`lp-box lp-tone-${tone}`} style={style}>
      <i className="lp-face lp-top" />
      <i className="lp-face lp-front">{children}</i>
      <i className="lp-face lp-right" />
    </div>
  );
}

function Vehicle({ kind, lane, clockwise, delay, duration, start }: { kind: "truck" | "van"; lane: number; clockwise: boolean; delay: number; duration: number; start: number }) {
  const style = {
    offsetPath: `path("${ring(lane, clockwise)}")`,
    offsetDistance: `${start}%`,
    animationDuration: `${duration}s`,
    animationDelay: `${delay}s`,
  } as CSSProperties;
  return (
    <div className={`lp-veh lp-veh--${kind}`} style={style}>
      <i className="lp-veh-shadow" />
      <i className="lp-veh-wheels" />
      <div className="lp-veh-body">
        <i className="lp-face lp-top" />
        <i className="lp-face lp-front" />
        <i className="lp-face lp-right" />
        <i className="lp-face lp-left" />
        <i className="lp-face lp-back" />
      </div>
      <div className="lp-veh-cab">
        <i className="lp-face lp-top" />
        <i className="lp-face lp-front" />
        <i className="lp-face lp-right" />
        <i className="lp-face lp-left" />
        <i className="lp-face lp-glass" />
      </div>
    </div>
  );
}

function Pin({ x, y, tone, label }: { x: number; y: number; tone: "amber" | "sky" | "green"; label: string }) {
  return (
    <div className="lp-pin" style={{ left: x, top: y }}>
      <i className={`lp-pin-ring lp-pin-ring--${tone}`} />
      <span className={`lp-pin-chip lp-pin-chip--${tone}`}><b />{label}</span>
    </div>
  );
}

export function HeroScene() {
  const { t } = useLocale();
  const wrap = useRef<HTMLDivElement>(null);
  const stage = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = wrap.current;
    const st = stage.current;
    if (!el || !st) return;
    if (matchMedia("(prefers-reduced-motion: reduce)").matches || matchMedia("(pointer: coarse)").matches) return;
    const move = (e: PointerEvent) => {
      const r = el.getBoundingClientRect();
      const nx = Math.max(-1, Math.min(1, ((e.clientX - r.left) / r.width - 0.5) * 2));
      const ny = Math.max(-1, Math.min(1, ((e.clientY - r.top) / r.height - 0.5) * 2));
      st.style.setProperty("--rz", `${-38 + nx * 7}deg`);
      st.style.setProperty("--rx", `${56 - ny * 6}deg`);
    };
    const leave = () => {
      st.style.setProperty("--rz", "-38deg");
      st.style.setProperty("--rx", "56deg");
    };
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerleave", leave);
    return () => {
      el.removeEventListener("pointermove", move);
      el.removeEventListener("pointerleave", leave);
    };
  }, []);

  return (
    <div className="lp-scene" ref={wrap} aria-hidden="true">
      <span className="lp-glow lp-glow--a" />
      <span className="lp-glow lp-glow--b" />
      <div className="lp-float-wrap">
        <div className="lp-stage" ref={stage} style={{ width: SIZE, height: SIZE }}>
          <i className="lp-drop" />
          {Array.from({ length: 9 }, (_, i) => (
            <i className="lp-slab" key={i} style={{ transform: `translateZ(${-(i + 1) * 2}px)`, filter: `brightness(${1 - i * 0.035})` }} />
          ))}
          <div className="lp-ground">
            <svg className="lp-roads" viewBox={`0 0 ${SIZE} ${SIZE}`}>
              <path className="lp-road-edge" d={ring(0, true)} />
              <path className="lp-road-fill" d={ring(0, true)} />
              <path className="lp-road-dash" d={ring(0, true)} />
              <path className="lp-road-edge" d="M220 90 V176" />
              <path className="lp-road-fill" d="M220 90 V176" />
              <rect className="lp-yard" x="150" y="166" width="140" height="22" rx="4" />
            </svg>
          </div>

          {/* depot and stores */}
          <Box x={150} y={116} w={140} d={56} h={62} tone="depot">
            <b className="lp-door" /><b className="lp-door" /><b className="lp-door" /><b className="lp-door" />
          </Box>
          <Box x={82} y={118} w={40} d={36} h={40} tone="a" />
          <Box x={326} y={120} w={40} d={34} h={54} tone="b" />
          <Box x={84} y={262} w={46} d={40} h={34} tone="c" />
          <Box x={322} y={262} w={44} d={42} h={64} tone="a" />
          <Box x={196} y={266} w={48} d={36} h={28} tone="b" />
          <Box x={142} y={216} w={26} d={26} h={22} tone="c" />
          <Box x={262} y={214} w={30} d={30} h={32} tone="a" />

          <Vehicle kind="truck" lane={9} clockwise delay={0} duration={16} start={8} />
          <Vehicle kind="van" lane={9} clockwise delay={-8} duration={16} start={58} />
          <Vehicle kind="truck" lane={-9} clockwise={false} delay={-4} duration={19} start={33} />

          <Pin x={102} y={136} tone="amber" label={t("Order placed")} />
          <Pin x={220} y={144} tone="sky" label={t("Plan locked")} />
          <Pin x={344} y={284} tone="green" label={t("Delivered")} />
        </div>
      </div>
    </div>
  );
}
