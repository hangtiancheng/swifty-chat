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

import { useQuery } from "@tanstack/react-query";

import { ApplyListDialog } from "@/components/apply-list-dialog";
import { groupAppliesQuery } from "@/service/queries";
import useAuthStore from "@/store/auth";

const GROUP_APPLY = 1;
const PENDING = 0;

interface GroupRequestsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function GroupRequestsDialog({
  open,
  onOpenChange,
}: GroupRequestsDialogProps) {
  const userId = useAuthStore((state) => state.userInfo.uuid);
  const applies = useQuery({ ...groupAppliesQuery(userId), enabled: open });

  const pending = (applies.data ?? []).filter(
    (apply) => apply.contact_type === GROUP_APPLY && apply.status === PENDING,
  );

  return (
    <ApplyListDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Group Join Requests"
      emptyLabel="No pending join requests"
      applies={pending}
      isPending={applies.isPending}
    />
  );
}
