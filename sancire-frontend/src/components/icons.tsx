import type { SVGProps } from "react";
type IconProps = SVGProps<SVGSVGElement> & { size?: number };
function Svg({ size = 20, ...props }: IconProps) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.6"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
      {...props}
    />
  );
}
export function IconMenu(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 7h14M5 12h10M5 17h14" />
    </Svg>
  );
}
export function IconSearch(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="11" cy="11" r="6.5" />
      <path d="M16 16l4 4" />
    </Svg>
  );
}
export function IconPlus(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 5v14M5 12h14" />
    </Svg>
  );
}
export function IconPin(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M9 4l6 2-1 6 4 3v2H6v-2l4-3-1-6zM12 17v3" />
    </Svg>
  );
}
export function IconTrash(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 7h14M10 7V5h4v2M8 7l1 12h6l1-12" />
    </Svg>
  );
}
export function IconPaperclip(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M8 12.5l6.5-6.5a3.2 3.2 0 114.5 4.5L9.5 20a4.5 4.5 0 11-6.4-6.4L13 3.8" />
    </Svg>
  );
}
export function IconMic(props: IconProps) {
  return (
    <Svg {...props}>
      <rect x="9" y="3" width="6" height="11" rx="3" />
      <path d="M6 11a6 6 0 0012 0M12 17v4" />
    </Svg>
  );
}
export function IconSend(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 12h12M13 6l6 6-6 6" />
    </Svg>
  );
}
export function IconSun(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 3v2M12 19v2M4.2 4.2l1.4 1.4M18.4 18.4l1.4 1.4M3 12h2M19 12h2M4.2 19.8l1.4-1.4M18.4 5.6l1.4-1.4" />
    </Svg>
  );
}
export function IconMoon(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M19 14.5A7.5 7.5 0 1111 4a6 6 0 008 10.5z" />
    </Svg>
  );
}
export function IconX(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M6 6l12 12M18 6L6 18" />
    </Svg>
  );
}
export function IconCopy(props: IconProps) {
  return (
    <Svg {...props}>
      <rect x="8" y="8" width="11" height="11" rx="2" />
      <path d="M5 16V5h11" />
    </Svg>
  );
}
export function IconDownload(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 4v12M7 11l5 5 5-5M5 20h14" />
    </Svg>
  );
}
export function IconSettings(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="12" r="3" />
      <path d="M12 4v2M12 18v2M4 12h2M18 12h2M6.3 6.3l1.4 1.4M16.3 16.3l1.4 1.4M6.3 17.7l1.4-1.4M16.3 7.7l1.4-1.4" />
    </Svg>
  );
}
export function IconLibrary(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 5h5v14H5zM14 5h5v14h-5z" />
    </Svg>
  );
}
export function IconFile(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M7 4h7l4 4v12H7zM14 4v4h4" />
    </Svg>
  );
}
export function IconCheck(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 12l5 5 9-9" />
    </Svg>
  );
}
export function IconExpand(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M9 5H5v4M15 5h4v4M5 15v4h4M19 15v4h-4" />
    </Svg>
  );
}
export function IconLogo(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 3v18M12 10L6 6M12 10l6-4M12 16l-5 4M12 16l5 4" />
    </Svg>
  );
}
export function IconStop(props: IconProps) {
  return (
    <Svg {...props}>
      <rect x="6" y="6" width="12" height="12" rx="2.5" fill="currentColor" stroke="none" />
    </Svg>
  );
}

export function IconChat(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M4 6.5A2.5 2.5 0 016.5 4h11A2.5 2.5 0 0120 6.5v7a2.5 2.5 0 01-2.5 2.5H10l-4.5 4v-4h-1A2.5 2.5 0 014 13.5z" />
      <path d="M8 8.5h8M8 11.5h5" />
    </Svg>
  );
}
export function IconGrid(props: IconProps) {
  return (
    <Svg {...props}>
      <rect x="4" y="4" width="7" height="7" rx="1.6" />
      <rect x="13" y="4" width="7" height="7" rx="1.6" />
      <rect x="4" y="13" width="7" height="7" rx="1.6" />
      <rect x="13" y="13" width="7" height="7" rx="1.6" />
    </Svg>
  );
}
export function IconShield(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 3.5l7 2.7v5.2c0 4.3-2.9 7.4-7 9.1-4.1-1.7-7-4.8-7-9.1V6.2z" />
      <path d="M9 12l2.2 2.2L15.5 10" />
    </Svg>
  );
}
export function IconSliders(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M5 7h14M5 17h14" />
      <circle cx="9.5" cy="7" r="2.2" />
      <circle cx="15" cy="17" r="2.2" />
    </Svg>
  );
}
export function IconUser(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="8.5" r="3.5" />
      <path d="M5.5 19.5c1-3.4 3.4-5 6.5-5s5.5 1.6 6.5 5" />
    </Svg>
  );
}
export function IconAlert(props: IconProps) {
  return (
    <Svg {...props}>
      <path d="M12 4.5l8 14H4z" />
      <path d="M12 10v4M12 16.6v.4" />
    </Svg>
  );
}
export function IconInfo(props: IconProps) {
  return (
    <Svg {...props}>
      <circle cx="12" cy="12" r="8" />
      <path d="M12 11v5M12 8.4v.3" />
    </Svg>
  );
}
