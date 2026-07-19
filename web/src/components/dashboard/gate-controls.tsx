"use client";

import { useState } from "react";
import { AlertTriangle, Check, Loader2 } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@/components/ui/popover";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  useCreateMaintenanceWindow,
  useRecordSignoff,
  useSignoffs,
} from "@/lib/queries";
import type { RingView } from "@/lib/types";

// Promotion-policy gate controls, shown inside the Seed/Promote dialogs when the
// target ring is gated. Each gate shows its live status and an inline action so
// a release engineer can satisfy it right here: open a maintenance window,
// record a QA/release Go-No-Go sign-off, or enter a change-request code.

export function gatesActive(target: RingView | undefined): boolean {
  const g = target?.gates;
  return !!g && (g.maintenance_window || g.qa_signoff || g.change_request);
}

export function GateChecklist({
  app,
  target,
  version,
  crCode,
  setCrCode,
}: {
  app: string;
  target: RingView;
  version: string;
  crCode: string;
  setCrCode: (v: string) => void;
}) {
  const g = target.gates;
  if (!gatesActive(target)) return null;
  return (
    <div className="space-y-3 rounded-lg border p-3">
      <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
        Promotion gates · {target.ring.label}
      </p>
      {g.maintenance_window && (
        <MaintenanceGate app={app} target={target} />
      )}
      {g.qa_signoff && (
        <SignoffGate app={app} ring={target.ring.name} version={version} />
      )}
      {g.change_request && (
        <ChangeRequestGate
          provider={g.change_request_provider}
          value={crCode}
          onChange={setCrCode}
        />
      )}
    </div>
  );
}

/** One gate row: status icon + label on the left, an inline action on the right. */
function GateRow({
  ok,
  label,
  detail,
  action,
}: {
  ok: boolean;
  label: string;
  detail?: string;
  action?: React.ReactNode;
}) {
  return (
    <div className="flex items-start justify-between gap-3">
      <span className="flex min-w-0 items-start gap-2 text-sm">
        {ok ? (
          <Check
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-status-good"
          />
        ) : (
          <AlertTriangle
            aria-hidden
            className="mt-0.5 size-4 shrink-0 text-status-warning"
          />
        )}
        <span className="min-w-0">
          <span className="font-medium">{label}</span>
          {detail && (
            <span className="block text-xs text-muted-foreground">{detail}</span>
          )}
        </span>
      </span>
      {action && <span className="shrink-0">{action}</span>}
    </div>
  );
}

function MaintenanceGate({ app, target }: { app: string; target: RingView }) {
  const open = target.gates.maintenance_window_open;
  return (
    <GateRow
      ok={open}
      label="Maintenance window"
      detail={open ? "A window is open now" : "No window open right now"}
      action={<OpenWindowPopover app={app} ring={target.ring.name} />}
    />
  );
}

function OpenWindowPopover({ app, ring }: { app: string; ring: string }) {
  const [open, setOpen] = useState(false);
  const [hours, setHours] = useState("2");
  const [reason, setReason] = useState("");
  const [engineer, setEngineer] = useState("");
  const create = useCreateMaintenanceWindow(app);

  const submit = () => {
    const now = new Date();
    const end = new Date(now.getTime() + Number(hours) * 3_600_000);
    create.mutate(
      {
        ring,
        starts_at: now.toISOString(),
        ends_at: end.toISOString(),
        reason: reason.trim(),
        created_by: engineer.trim(),
      },
      { onSuccess: () => setOpen(false) },
    );
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          Open window
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-3">
        <p className="text-sm font-medium">Open a maintenance window</p>
        <div className="space-y-1.5">
          <Label htmlFor="mw-hours">Duration</Label>
          <Select value={hours} onValueChange={setHours}>
            <SelectTrigger id="mw-hours" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {["1", "2", "4", "8"].map((h) => (
                <SelectItem key={h} value={h}>
                  {h} hour{h === "1" ? "" : "s"} from now
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="mw-reason">Reason</Label>
          <Input
            id="mw-reason"
            value={reason}
            onChange={(e) => setReason(e.target.value)}
            placeholder="e.g. scheduled release"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="mw-eng">Opened by</Label>
          <Input
            id="mw-eng"
            value={engineer}
            onChange={(e) => setEngineer(e.target.value)}
            placeholder="your name"
          />
        </div>
        <Button
          type="button"
          size="sm"
          className="w-full"
          disabled={create.isPending}
          onClick={submit}
        >
          {create.isPending && (
            <Loader2 aria-hidden className="size-4 animate-spin" />
          )}
          Open for {hours}h
        </Button>
      </PopoverContent>
    </Popover>
  );
}

function SignoffGate({
  app,
  ring,
  version,
}: {
  app: string;
  ring: string;
  version: string;
}) {
  const { data: signoffs } = useSignoffs(app);
  const current = signoffs?.find(
    (s) => s.ring === ring && s.version === version,
  );
  const isGo = current?.decision === "go";
  const detail = !version
    ? "Select a version to sign off"
    : current
      ? isGo
        ? `GO by ${current.engineer}${current.qa_status ? ` · QA ${current.qa_status}` : ""}`
        : `NO-GO by ${current.engineer}`
      : "Not yet signed off for this version";

  return (
    <GateRow
      ok={isGo}
      label="QA / release sign-off"
      detail={detail}
      action={
        version ? (
          <SignoffPopover app={app} ring={ring} version={version} />
        ) : undefined
      }
    />
  );
}

function SignoffPopover({
  app,
  ring,
  version,
}: {
  app: string;
  ring: string;
  version: string;
}) {
  const [open, setOpen] = useState(false);
  const [engineer, setEngineer] = useState("");
  const [qaStatus, setQaStatus] = useState("passed");
  const [note, setNote] = useState("");
  const record = useRecordSignoff(app);

  const submit = (decision: "go" | "no_go") => {
    record.mutate(
      { ring, version, decision, engineer: engineer.trim(), qa_status: qaStatus, note: note.trim() },
      { onSuccess: () => setOpen(false) },
    );
  };

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button type="button" variant="outline" size="sm">
          Record
        </Button>
      </PopoverTrigger>
      <PopoverContent align="end" className="w-72 space-y-3">
        <div>
          <p className="text-sm font-medium">Go / No-Go sign-off</p>
          <p className="text-xs text-muted-foreground">
            <span className="font-mono">{version}</span> → {ring}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="so-eng">Release engineer</Label>
          <Input
            id="so-eng"
            value={engineer}
            onChange={(e) => setEngineer(e.target.value)}
            placeholder="your name"
          />
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="so-qa">QA status</Label>
          <Select value={qaStatus} onValueChange={setQaStatus}>
            <SelectTrigger id="so-qa" className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {["passed", "passed-with-waivers", "not-run"].map((s) => (
                <SelectItem key={s} value={s}>
                  {s}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="so-note">Note (optional)</Label>
          <Input
            id="so-note"
            value={note}
            onChange={(e) => setNote(e.target.value)}
            placeholder="context for this decision"
          />
        </div>
        <div className="flex gap-2">
          <Button
            type="button"
            size="sm"
            className="flex-1"
            disabled={!engineer.trim() || record.isPending}
            onClick={() => submit("go")}
          >
            GO
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            className="flex-1 text-status-critical hover:text-status-critical"
            disabled={!engineer.trim() || record.isPending}
            onClick={() => submit("no_go")}
          >
            NO-GO
          </Button>
        </div>
      </PopoverContent>
    </Popover>
  );
}

function ChangeRequestGate({
  provider,
  value,
  onChange,
}: {
  provider?: string;
  value: string;
  onChange: (v: string) => void;
}) {
  const ok = value.trim().length > 0;
  return (
    <div className="space-y-1.5">
      <GateRow
        ok={ok}
        label="Change-request code"
        detail={
          provider === "jira"
            ? "Validated against JIRA — or use “test” for a demo"
            : "Enter the approved change code (or “test” for a demo)"
        }
      />
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="e.g. CR-1234 (or test)"
        autoComplete="off"
        className="font-mono"
      />
    </div>
  );
}
