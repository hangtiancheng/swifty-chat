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
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { xor } from "es-toolkit";
import { ChartBar } from "lucide-react";
import { useState } from "react";
import { useNavigate } from "react-router-dom";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { useWindowedRows } from "@/lib/use-windowed-rows";
import { group, user } from "@/service/api";
import { adminGroupsQuery, adminUsersQuery, keys } from "@/service/queries";
import useAuthStore from "@/store/auth";
import { showToast } from "@/utils/toast";

const ROW_HEIGHT = 44;
const BANNED = 1;

interface BulkAction {
  run: (ids: string[]) => Promise<void>;
  done: string;
  staleKey: readonly unknown[];
}

function SelectionBar({
  count,
  children,
}: {
  count: number;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-wrap items-center gap-2">
      <span className="text-muted-foreground text-xs tabular-nums">
        <NumberFlow value={count} /> selected
      </span>
      {children}
    </div>
  );
}

export default function Manager() {
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const userId = useAuthStore((state) => state.userInfo.uuid);

  const [selectedUsers, setSelectedUsers] = useState<string[]>([]);
  const [selectedGroups, setSelectedGroups] = useState<string[]>([]);

  const users = useQuery(adminUsersQuery(userId));
  const groups = useQuery(adminGroupsQuery());

  const bulk = useMutation({
    mutationFn: ({ action, ids }: { action: BulkAction; ids: string[] }) =>
      action.run(ids),
    onSuccess: (_result, { action }) => {
      showToast(action.done, "success");
      void queryClient.invalidateQueries({ queryKey: action.staleKey });
      setSelectedUsers([]);
      setSelectedGroups([]);
    },
  });

  const userRows = users.data ?? [];
  const groupRows = groups.data ?? [];

  const {
    scrollRef: usersScrollRef,
    items: userItems,
    paddingTop: usersPaddingTop,
    paddingBottom: usersPaddingBottom,
  } = useWindowedRows(userRows.length, ROW_HEIGHT);
  const {
    scrollRef: groupsScrollRef,
    items: groupItems,
    paddingTop: groupsPaddingTop,
    paddingBottom: groupsPaddingBottom,
  } = useWindowedRows(groupRows.length, ROW_HEIGHT);

  const runOnUsers = (action: BulkAction) => {
    if (selectedUsers.length === 0) {
      showToast("Select at least one row", "warning");
      return;
    }
    bulk.mutate({ action, ids: selectedUsers });
  };

  const runOnGroups = (action: BulkAction) => {
    if (selectedGroups.length === 0) {
      showToast("Select at least one row", "warning");
      return;
    }
    bulk.mutate({ action, ids: selectedGroups });
  };

  const userActions: Record<string, BulkAction> = {
    enable: {
      run: user.enable,
      done: "Accounts enabled",
      staleKey: keys.users.all,
    },
    disable: {
      run: user.disable,
      done: "Accounts disabled",
      staleKey: keys.users.all,
    },
    remove: {
      run: user.remove,
      done: "Accounts deleted",
      staleKey: keys.users.all,
    },
    promote: {
      run: (ids) => user.setAdmin(ids, 1),
      done: "Admin granted",
      staleKey: keys.users.all,
    },
    demote: {
      run: (ids) => user.setAdmin(ids, 0),
      done: "Admin revoked",
      staleKey: keys.users.all,
    },
  };

  const groupActions: Record<string, BulkAction> = {
    enable: {
      run: (ids) => group.setStatus(ids, 0),
      done: "Groups enabled",
      staleKey: keys.groups.all,
    },
    disable: {
      run: (ids) => group.setStatus(ids, BANNED),
      done: "Groups disabled",
      staleKey: keys.groups.all,
    },
    remove: {
      run: group.removeAll,
      done: "Groups deleted",
      staleKey: keys.groups.all,
    },
  };

  const allUsersSelected =
    userRows.length > 0 && selectedUsers.length === userRows.length;
  const allGroupsSelected =
    groupRows.length > 0 && selectedGroups.length === groupRows.length;

  return (
    <div className="flex min-w-0 flex-1 flex-col gap-3 p-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold">Administration</h2>
        <Button
          variant="outline"
          size="sm"
          onClick={() => navigate("/dashboard")}
        >
          <ChartBar className="size-3.5" />
          Cache Dashboard
        </Button>
      </div>

      <Tabs defaultValue="users" className="min-h-0 flex-1">
        <TabsList className="w-64">
          <TabsTrigger value="users">Users</TabsTrigger>
          <TabsTrigger value="groups">Groups</TabsTrigger>
        </TabsList>

        <TabsContent value="users" className="flex min-h-0 flex-col gap-2">
          <SelectionBar count={selectedUsers.length}>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnUsers(userActions.enable)}
            >
              Enable
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnUsers(userActions.disable)}
            >
              Disable
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnUsers(userActions.promote)}
            >
              Grant Admin
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnUsers(userActions.demote)}
            >
              Revoke Admin
            </Button>
            <Button
              size="sm"
              variant="destructive"
              onClick={() => runOnUsers(userActions.remove)}
            >
              Delete
            </Button>
          </SelectionBar>

          <div
            ref={usersScrollRef}
            className="border-border min-h-0 flex-1 overflow-y-auto rounded-md border"
          >
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={allUsersSelected}
                      onCheckedChange={(checked) =>
                        setSelectedUsers(
                          checked === true
                            ? userRows.map((row) => row.uuid)
                            : [],
                        )
                      }
                    />
                  </TableHead>
                  <TableHead>Nickname</TableHead>
                  <TableHead>Phone</TableHead>
                  <TableHead>UUID</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {usersPaddingTop > 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      style={{ height: usersPaddingTop }}
                    />
                  </TableRow>
                )}
                {userItems.map((item) => {
                  const row = userRows[item.index];
                  return (
                    <TableRow
                      key={row.uuid}
                      className="cursor-pointer"
                      onClick={() =>
                        setSelectedUsers((previous) =>
                          xor(previous, [row.uuid]),
                        )
                      }
                    >
                      <TableCell>
                        <Checkbox
                          checked={selectedUsers.includes(row.uuid)}
                          onCheckedChange={() =>
                            setSelectedUsers((previous) =>
                              xor(previous, [row.uuid]),
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>{row.nickname}</TableCell>
                      <TableCell>{row.telephone}</TableCell>
                      <TableCell className="font-mono text-xs">
                        {row.uuid}
                      </TableCell>
                      <TableCell className="flex gap-1">
                        {row.is_admin === 1 && <Badge>admin</Badge>}
                        {row.status === BANNED && (
                          <Badge variant="destructive">banned</Badge>
                        )}
                        {row.is_deleted && (
                          <Badge variant="secondary">deleted</Badge>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
                {usersPaddingBottom > 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      style={{ height: usersPaddingBottom }}
                    />
                  </TableRow>
                )}
              </TableBody>
            </Table>
            {users.isPending && (
              <p className="text-muted-foreground animate-pulse p-4 text-sm">
                Loading users…
              </p>
            )}
          </div>
        </TabsContent>

        <TabsContent value="groups" className="flex min-h-0 flex-col gap-2">
          <SelectionBar count={selectedGroups.length}>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnGroups(groupActions.enable)}
            >
              Enable
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => runOnGroups(groupActions.disable)}
            >
              Disable
            </Button>
            <Button
              size="sm"
              variant="destructive"
              onClick={() => runOnGroups(groupActions.remove)}
            >
              Delete
            </Button>
          </SelectionBar>

          <div
            ref={groupsScrollRef}
            className="border-border min-h-0 flex-1 overflow-y-auto rounded-md border"
          >
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead className="w-10">
                    <Checkbox
                      checked={allGroupsSelected}
                      onCheckedChange={(checked) =>
                        setSelectedGroups(
                          checked === true
                            ? groupRows.map((row) => row.group_id)
                            : [],
                        )
                      }
                    />
                  </TableHead>
                  <TableHead>Name</TableHead>
                  <TableHead>Owner</TableHead>
                  <TableHead>Members</TableHead>
                  <TableHead>Status</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {groupsPaddingTop > 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      style={{ height: groupsPaddingTop }}
                    />
                  </TableRow>
                )}
                {groupItems.map((item) => {
                  const row = groupRows[item.index];
                  return (
                    <TableRow
                      key={row.group_id}
                      className="cursor-pointer"
                      onClick={() =>
                        setSelectedGroups((previous) =>
                          xor(previous, [row.group_id]),
                        )
                      }
                    >
                      <TableCell>
                        <Checkbox
                          checked={selectedGroups.includes(row.group_id)}
                          onCheckedChange={() =>
                            setSelectedGroups((previous) =>
                              xor(previous, [row.group_id]),
                            )
                          }
                        />
                      </TableCell>
                      <TableCell>{row.name}</TableCell>
                      <TableCell className="font-mono text-xs">
                        {row.owner_id}
                      </TableCell>
                      <TableCell className="tabular-nums">
                        {row.member_cnt}
                      </TableCell>
                      <TableCell className="flex gap-1">
                        {row.status === BANNED && (
                          <Badge variant="destructive">disabled</Badge>
                        )}
                        {row.is_deleted && (
                          <Badge variant="secondary">deleted</Badge>
                        )}
                      </TableCell>
                    </TableRow>
                  );
                })}
                {groupsPaddingBottom > 0 && (
                  <TableRow>
                    <TableCell
                      colSpan={5}
                      style={{ height: groupsPaddingBottom }}
                    />
                  </TableRow>
                )}
              </TableBody>
            </Table>
            {groups.isPending && (
              <p className="text-muted-foreground animate-pulse p-4 text-sm">
                Loading groups…
              </p>
            )}
          </div>
        </TabsContent>
      </Tabs>
    </div>
  );
}
