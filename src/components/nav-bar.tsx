/**
 * Copyright (c) 2026 hangtiancheng
 *
 * Permission is hereby granted, free of charge, to any person obtaining a copy
 * of this software and associated documentation files (the "Software"), to deal
 * in the Software without restriction, including without limitation the rights
 * to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
 * copies of the Software, and to permit persons to whom the Software is
 * furnished to do so, subject to the following conditions:
 *
 * The above copyright notice and this permission notice shall be included in
 * all copies or substantial portions of the Software.
 *
 * THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
 * IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
 * FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
 * AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
 * LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
 * OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
 * SOFTWARE.
 */

import {
  LogOut,
  MessageSquare,
  Moon,
  Settings,
  Sun,
  User,
  Users,
} from "lucide-react";
import type { LucideIcon } from "lucide-react";
import { motion } from "motion/react";
import { useTheme } from "next-themes";
import { useLocation, useNavigate } from "react-router-dom";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import useAuthStore from "@/store/auth";
import { performLogout } from "@/utils/logout";

interface RailItem {
  label: string;
  path: string;
  icon: LucideIcon;
}

const NAV_ITEMS: RailItem[] = [
  { label: "Sessions", path: "/chat/sessions", icon: MessageSquare },
  { label: "Contacts", path: "/chat/contacts", icon: Users },
  { label: "Profile", path: "/chat/profile", icon: User },
];

/** An open conversation (`/chat/:id`) still belongs to the Sessions section. */
function activeSection(pathname: string): string {
  if (pathname.startsWith("/chat/contacts")) return "/chat/contacts";
  if (pathname.startsWith("/chat/profile")) return "/chat/profile";
  if (pathname.startsWith("/chat")) return "/chat/sessions";
  return pathname;
}

interface RailButtonProps {
  label: string;
  icon: LucideIcon;
  onClick: () => void;
  active?: boolean;
  destructive?: boolean;
}

function RailButton({
  label,
  icon: Icon,
  onClick,
  active,
  destructive,
}: RailButtonProps) {
  return (
    <Tooltip>
      <TooltipTrigger
        render={
          <Button
            variant="ghost"
            size="icon"
            aria-label={label}
            aria-current={active ? "page" : undefined}
            onClick={onClick}
            className={cn(
              "relative transition-colors duration-200",
              destructive
                ? "text-destructive hover:bg-destructive/10 hover:text-destructive"
                : active
                  ? "text-primary hover:text-primary"
                  : "text-muted-foreground hover:bg-accent hover:text-accent-foreground",
            )}
          />
        }
      >
        {active && (
          <motion.span
            layoutId="nav-active-section"
            className="bg-primary/12 absolute inset-0 rounded-md"
            transition={{ type: "spring", stiffness: 380, damping: 30 }}
          />
        )}
        <Icon className="relative size-5" />
      </TooltipTrigger>
      <TooltipContent side="right">{label}</TooltipContent>
    </Tooltip>
  );
}

export function NavBar() {
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const userInfo = useAuthStore((state) => state.userInfo);
  const { resolvedTheme, setTheme } = useTheme();

  const current = activeSection(pathname);
  const isDark = resolvedTheme === "dark";

  const signOut = async () => {
    await performLogout();
    navigate("/login");
  };

  return (
    <nav className="border-border bg-muted/50 flex h-full w-16 shrink-0 flex-col items-center border-r py-4">
      <Avatar className="ring-primary/30 ring-offset-card size-10 ring-2 ring-offset-2 transition-transform duration-200 hover:scale-105">
        <AvatarImage src={userInfo.avatar} alt={userInfo.nickname} />
        <AvatarFallback>
          {userInfo.nickname.charAt(0).toUpperCase() || "U"}
        </AvatarFallback>
      </Avatar>

      <div className="mt-6 flex flex-col items-center gap-1">
        {NAV_ITEMS.map((item) => (
          <RailButton
            key={item.path}
            label={item.label}
            icon={item.icon}
            active={current === item.path}
            onClick={() => navigate(item.path)}
          />
        ))}
      </div>

      <div className="flex-1" />

      <div className="flex flex-col items-center gap-1">
        <div className="bg-border mb-2 h-px w-8" aria-hidden="true" />
        <RailButton
          label={isDark ? "Light mode" : "Dark mode"}
          icon={isDark ? Sun : Moon}
          onClick={() => setTheme(isDark ? "light" : "dark")}
        />
        {userInfo.is_admin === 1 && (
          <RailButton
            label="Admin"
            icon={Settings}
            active={current === "/manager"}
            onClick={() => navigate("/manager")}
          />
        )}
        <RailButton
          label="Sign Out"
          icon={LogOut}
          onClick={signOut}
          destructive
        />
      </div>
    </nav>
  );
}
