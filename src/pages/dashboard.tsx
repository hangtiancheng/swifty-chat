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

import NumberFlow from "@number-flow/react";
import { Trash2 } from "lucide-react";
import { motion } from "motion/react";
import { useEffect, useMemo } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { wsUrl } from "@/env";
import { useWindowedRows } from "@/lib/use-windowed-rows";
import useDashboardStore, {
  type DashboardStatus,
  type GroupSnapshot,
} from "@/store/dashboard";
import { formatExpire, formatSize } from "@/utils/format";

const ROW_HEIGHT = 36;
const DASHBOARD_WS = `${wsUrl}/dashboard/ws`;

interface FlatRow {
  group: string;
  key: string;
  size: number;
  level: number;
  expire_at: number;
}

function flatten(groups: GroupSnapshot[]): FlatRow[] {
  const rows: FlatRow[] = [];
  for (const group of groups) {
    for (const entry of group.entries ?? []) {
      rows.push({
        group: group.name,
        key: entry.key,
        size: entry.size,
        level: entry.level,
        expire_at: entry.expire_at,
      });
    }
  }
  return rows;
}

const STATUS_STYLE: Record<DashboardStatus, string> = {
  connected: "bg-emerald-500",
  connecting: "bg-amber-500",
  disconnected: "bg-destructive",
};

function StatusBadge({ status }: { status: DashboardStatus }) {
  return (
    <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
      <motion.span
        className={`size-2 rounded-full ${STATUS_STYLE[status]}`}
        animate={
          status === "connecting" ? { opacity: [1, 0.3, 1] } : { opacity: 1 }
        }
        transition={
          status === "connecting"
            ? { duration: 1.2, repeat: Infinity }
            : { duration: 0.2 }
        }
      />
      {status}
    </span>
  );
}

export default function Dashboard() {
  const navigate = useNavigate();
  // Selector subscriptions replace the old 500 ms poll of the store snapshot.
  const groups = useDashboardStore((state) => state.groups);
  const status = useDashboardStore((state) => state.status);

  useEffect(() => {
    const store = useDashboardStore.getState();
    store.connect(DASHBOARD_WS);
    return () => store.disconnect();
  }, []);

  const rows = useMemo(() => flatten(groups), [groups]);
  const { scrollRef, items, paddingTop, paddingBottom } = useWindowedRows(
    rows.length,
    ROW_HEIGHT,
  );

  return (
    <div className="bg-background flex min-h-screen flex-col gap-3 p-6">
      <div className="flex items-center justify-between">
        <div className="flex items-baseline gap-3">
          <h1 className="text-lg font-semibold">Cache Dashboard</h1>
          <span className="text-muted-foreground text-xs tabular-nums">
            <NumberFlow value={rows.length} /> entries across {groups.length}{" "}
            groups
          </span>
        </div>
        <div className="flex items-center gap-3">
          <StatusBadge status={status} />
          <Button
            variant="outline"
            size="sm"
            onClick={() => useDashboardStore.getState().connect(DASHBOARD_WS)}
          >
            Reconnect
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => navigate("/manager")}
          >
            Back
          </Button>
        </div>
      </div>

      <div
        ref={scrollRef}
        className="border-border min-h-0 flex-1 overflow-y-auto rounded-md border"
      >
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Group</TableHead>
              <TableHead>Key</TableHead>
              <TableHead>Size</TableHead>
              <TableHead>Level</TableHead>
              <TableHead>Expires</TableHead>
              <TableHead className="w-10" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {paddingTop > 0 && (
              <TableRow>
                <TableCell colSpan={6} style={{ height: paddingTop }} />
              </TableRow>
            )}
            {items.map((item) => {
              const row = rows[item.index];
              return (
                <TableRow key={`${row.group}:${row.key}`}>
                  <TableCell>
                    <Badge variant="secondary">{row.group}</Badge>
                  </TableCell>
                  <TableCell className="font-mono text-xs">{row.key}</TableCell>
                  <TableCell className="tabular-nums">
                    {formatSize(row.size)}
                  </TableCell>
                  <TableCell className="tabular-nums">{row.level}</TableCell>
                  <TableCell className="text-muted-foreground text-xs">
                    {formatExpire(row.expire_at)}
                  </TableCell>
                  <TableCell>
                    <Button
                      variant="ghost"
                      size="icon"
                      aria-label={`Delete ${row.key}`}
                      className="text-destructive size-7"
                      onClick={() =>
                        useDashboardStore
                          .getState()
                          .deleteKey(row.group, row.key)
                      }
                    >
                      <Trash2 className="size-3.5" />
                    </Button>
                  </TableCell>
                </TableRow>
              );
            })}
            {paddingBottom > 0 && (
              <TableRow>
                <TableCell colSpan={6} style={{ height: paddingBottom }} />
              </TableRow>
            )}
          </TableBody>
        </Table>
        {rows.length === 0 && (
          <p className="text-muted-foreground p-6 text-center text-sm">
            {status === "connected" ? "Cache is empty" : "Not connected"}
          </p>
        )}
      </div>
    </div>
  );
}
