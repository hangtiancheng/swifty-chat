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
