import Linkify from "linkify-react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import type { ContactInfo } from "@/service/schemas";

const linkOptions = {
  target: "_blank",
  rel: "noreferrer",
  className: "text-primary underline underline-offset-2",
};

function InfoRow({ label, value }: { label: string; value: string | number }) {
  return (
    <div className="border-border flex justify-between gap-4 border-b py-1.5">
      <span className="text-muted-foreground shrink-0">{label}</span>
      <span className="text-foreground truncate">{value}</span>
    </div>
  );
}

interface ContactDetailDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  contact: ContactInfo | undefined;
  isGroup: boolean;
}

export function ContactDetailDialog({
  open,
  onOpenChange,
  contact,
  isGroup,
}: ContactDetailDialogProps) {
  const freeText = isGroup
    ? contact?.contact_notice
    : contact?.contact_signature;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{isGroup ? "Group Info" : "User Profile"}</DialogTitle>
        </DialogHeader>

        {contact && (
          <div className="flex flex-col gap-0.5 text-sm">
            <InfoRow label="ID" value={contact.contact_id} />
            <InfoRow label="Name" value={contact.contact_name} />
            {isGroup ? (
              <>
                <InfoRow label="Members" value={contact.contact_member_cnt} />
                <InfoRow label="Owner" value={contact.contact_owner_id} />
                <InfoRow
                  label="Join Mode"
                  value={
                    contact.contact_add_mode === 0
                      ? "Direct Join"
                      : "Owner Approval"
                  }
                />
              </>
            ) : (
              <>
                <InfoRow
                  label="Gender"
                  value={contact.contact_gender === 0 ? "Male" : "Female"}
                />
                <InfoRow label="Phone" value={contact.contact_phone} />
                <InfoRow label="Email" value={contact.contact_email} />
                <InfoRow label="Birthday" value={contact.contact_birthday} />
              </>
            )}
            <div className="py-1.5">
              <span className="text-muted-foreground">
                {isGroup ? "Notice" : "Signature"}
              </span>
              <Linkify
                as="p"
                options={linkOptions}
                className="text-foreground mt-1 break-words"
              >
                {freeText || "—"}
              </Linkify>
            </div>
          </div>
        )}

        <DialogFooter>
          <Button variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
