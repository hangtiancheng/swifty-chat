import { useQuery } from "@tanstack/react-query";
import { Mic, MicOff, Phone, PhoneOff, Volume2, VolumeX } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { callersQuery } from "@/service/queries";
import useAuthStore from "@/store/auth";
import useCallStore, { toggleMicrophone } from "@/store/call";
import type { CallPeer } from "@/utils/rtc";

function formatDuration(totalSeconds: number): string {
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, "0")}`;
}

/** Ticks a clock rather than the elapsed count, so no state is set on mount. */
function useElapsedSeconds(since: number | null): number {
  const [now, setNow] = useState(() => Date.now());

  useEffect(() => {
    if (!since) return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [since]);

  return since ? Math.max(0, Math.floor((now - since) / 1000)) : 0;
}

function useAttachedStream(stream: MediaStream | null) {
  const ref = useRef<HTMLVideoElement | null>(null);

  useEffect(() => {
    if (ref.current) ref.current.srcObject = stream;
  }, [stream]);

  return ref;
}

function VideoTile({
  stream,
  label,
  muted,
}: {
  stream: MediaStream | null;
  label: string;
  muted?: boolean;
}) {
  const ref = useAttachedStream(stream);

  return (
    <div className="bg-muted relative aspect-video overflow-hidden rounded-lg">
      <video
        ref={ref}
        autoPlay
        playsInline
        muted={muted}
        className="size-full object-cover"
      />
      <span className="absolute bottom-1 left-2 truncate text-[10px] font-medium text-white drop-shadow">
        {label}
      </span>
    </div>
  );
}

/** Remote audio needs an element of its own to actually play. */
function AudioTile({ peer, muted }: { peer: CallPeer; muted: boolean }) {
  const ref = useRef<HTMLAudioElement | null>(null);

  useEffect(() => {
    if (ref.current) ref.current.srcObject = peer.stream;
  }, [peer.stream]);

  return <audio ref={ref} autoPlay muted={muted} />;
}

export function CallDialog() {
  const phase = useCallStore((state) => state.phase);
  const media = useCallStore((state) => state.media);
  const kind = useCallStore((state) => state.kind);
  const title = useCallStore((state) => state.title);
  const roomId = useCallStore((state) => state.roomId);
  const incoming = useCallStore((state) => state.incoming);
  const localStream = useCallStore((state) => state.localStream);
  const peers = useCallStore((state) => state.peers);
  const microphoneOn = useCallStore((state) => state.microphoneOn);
  const connectedAt = useCallStore((state) => state.connectedAt);
  const accept = useCallStore((state) => state.accept);
  const decline = useCallStore((state) => state.decline);
  const hangUp = useCallStore((state) => state.hangUp);

  const userId = useAuthStore((state) => state.userInfo.uuid);
  const [mutedPeers, setMutedPeers] = useState<ReadonlySet<string>>(new Set());
  const elapsed = useElapsedSeconds(connectedAt);

  // Who is already talking, so a group invite can be judged before joining.
  const callers = useQuery({
    ...callersQuery(roomId, userId),
    enabled: Boolean(roomId && userId) && kind === "group",
  });

  const toggleMuted = (peerId: string) =>
    setMutedPeers((current) => {
      const next = new Set(current);
      if (!next.delete(peerId)) next.add(peerId);
      return next;
    });

  const mediaLabel = media === "audio" ? "Audio" : "Video";
  const heading =
    phase === "ringing"
      ? `Incoming ${mediaLabel.toLowerCase()} call`
      : phase === "active"
        ? formatDuration(elapsed)
        : `Calling ${title || "…"}`;

  return (
    <Dialog
      open={phase !== "idle"}
      onOpenChange={(next) => {
        if (next) return;
        if (phase === "ringing") decline();
        else hangUp();
      }}
    >
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{heading}</DialogTitle>
          <DialogDescription>
            {kind === "group" ? "Group" : "One-to-one"}{" "}
            {mediaLabel.toLowerCase()} call{title ? ` with ${title}` : ""}
          </DialogDescription>
        </DialogHeader>

        {phase === "ringing" ? (
          <div className="flex flex-col items-center gap-3 py-4">
            <Avatar className="size-20">
              <AvatarImage src={incoming?.avatar} alt={title} />
              <AvatarFallback>{title.charAt(0) || "?"}</AvatarFallback>
            </Avatar>
            <p className="text-foreground text-sm font-medium">{title}</p>
            {kind === "group" && (
              <p className="text-muted-foreground text-xs">
                {callers.data?.length
                  ? `${callers.data.length} already in this call`
                  : "Nobody has joined yet"}
              </p>
            )}
          </div>
        ) : media === "video" ? (
          <div
            className={cn(
              "grid gap-2",
              peers.length > 1 ? "grid-cols-3" : "grid-cols-2",
            )}
          >
            {peers.map((peer) => (
              <VideoTile
                key={peer.id}
                stream={peer.stream}
                label={peer.name}
                muted={mutedPeers.has(peer.id)}
              />
            ))}
            <VideoTile stream={localStream} label="You" muted />
            {peers.length === 0 && (
              <p className="text-muted-foreground col-span-full text-center text-xs">
                Waiting for someone to join…
              </p>
            )}
          </div>
        ) : (
          <div className="flex flex-wrap items-center justify-center gap-4 py-4">
            {peers.map((peer) => (
              <div key={peer.id} className="flex flex-col items-center gap-1.5">
                <AudioTile peer={peer} muted={mutedPeers.has(peer.id)} />
                <Avatar className="size-14">
                  <AvatarFallback>
                    {peer.name.charAt(0).toUpperCase() || "?"}
                  </AvatarFallback>
                </Avatar>
                <span className="text-muted-foreground max-w-24 truncate text-xs">
                  {peer.name}
                </span>
                <Button
                  variant="ghost"
                  size="icon"
                  aria-label={
                    mutedPeers.has(peer.id) ? "Unmute peer" : "Mute peer"
                  }
                  onClick={() => toggleMuted(peer.id)}
                >
                  {mutedPeers.has(peer.id) ? (
                    <VolumeX className="size-4" />
                  ) : (
                    <Volume2 className="size-4" />
                  )}
                </Button>
              </div>
            ))}
            {peers.length === 0 && (
              <p className="text-muted-foreground text-xs">
                Waiting for someone to join…
              </p>
            )}
          </div>
        )}

        <div className="flex justify-center gap-2">
          {phase === "ringing" ? (
            <>
              <Button size="sm" onClick={accept}>
                <Phone className="size-3.5" />
                Accept
              </Button>
              <Button size="sm" variant="destructive" onClick={decline}>
                <PhoneOff className="size-3.5" />
                Decline
              </Button>
            </>
          ) : (
            <>
              <Button
                size="sm"
                variant="outline"
                onClick={toggleMicrophone}
                aria-label={
                  microphoneOn ? "Mute microphone" : "Unmute microphone"
                }
              >
                {microphoneOn ? (
                  <Mic className="size-3.5" />
                ) : (
                  <MicOff className="size-3.5" />
                )}
                {microphoneOn ? "Mute" : "Unmute"}
              </Button>
              <Button size="sm" variant="destructive" onClick={hangUp}>
                <PhoneOff className="size-3.5" />
                Hang Up
              </Button>
            </>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}
