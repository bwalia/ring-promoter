"use client";

import { useState } from "react";
import {
  AlertTriangle,
  Check,
  ChevronsUpDown,
  GitBranch,
  Hash,
  Loader2,
  Tag,
} from "lucide-react";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
} from "@/components/ui/command";
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
import { Skeleton } from "@/components/ui/skeleton";
import {
  usePromoteMutation,
  useRollbackMutation,
  useSeedMutation,
  useVersions,
} from "@/lib/queries";
import { useUiStore } from "@/lib/ui-store";
import type { AppVersion, RingView } from "@/lib/types";
import { cn } from "@/lib/utils";

/**
 * The seed / promote / rollback dialogs for the selected app. All are driven
 * by uiStore.pendingAction so ring-card buttons and the command palette share
 * one implementation. Destructive/irreversible operations always confirm.
 */
export function ActionDialogs({
  app,
  rings,
}: {
  app: string;
  rings: RingView[];
}) {
  const pending = useUiStore((s) => s.pendingAction);
  const setPending = useUiStore((s) => s.setPendingAction);
  const close = () => setPending(null);

  return (
    <>
      {pending?.type === "seed" && (
        <SeedDialog
          app={app}
          rings={rings}
          initialRing={pending.ring}
          onClose={close}
        />
      )}
      <PromoteDialog
        app={app}
        rings={rings}
        fromRing={pending?.type === "promote" ? pending.fromRing : null}
        onClose={close}
      />
      <RollbackDialog
        app={app}
        rings={rings}
        ring={pending?.type === "rollback" ? pending.ring : null}
        onClose={close}
      />
    </>
  );
}

function SeedDialog({
  app,
  rings,
  initialRing,
  onClose,
}: {
  app: string;
  rings: RingView[];
  initialRing?: string;
  onClose: () => void;
}) {
  // Mounted fresh on every open (see ActionDialogs), so initial state is enough.
  const configured = rings.filter((r) => r.configured);
  const [ring, setRing] = useState(
    initialRing ?? configured[0]?.ring.name ?? "",
  );
  const [version, setVersion] = useState("");
  const seed = useSeedMutation(app);
  const versionsQuery = useVersions(app);

  const target = configured.find((r) => r.ring.name === ring);
  const replaces = target?.current_version;

  const submit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!ring || !version.trim() || seed.isPending) return;
    seed.mutate(
      { ring, version: version.trim() },
      { onSuccess: onClose },
    );
  };

  return (
    <Dialog open onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-md">
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>Seed a version</DialogTitle>
            <DialogDescription>
              Deploy a specific version (image tag, branch or commit) directly
              into one ring of <span className="font-medium">{app}</span>.
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-2">
            <Label htmlFor="seed-ring">Ring</Label>
            <Select value={ring} onValueChange={setRing}>
              <SelectTrigger id="seed-ring" className="w-full">
                <SelectValue placeholder="Choose a ring" />
              </SelectTrigger>
              <SelectContent>
                {configured.map((r) => (
                  <SelectItem key={r.ring.name} value={r.ring.name}>
                    {r.ring.label} ({r.ring.name})
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          <div className="space-y-2">
            <Label htmlFor="seed-version">Version</Label>
            {versionsQuery.isPending ? (
              <Skeleton className="h-9 w-full" />
            ) : versionsQuery.data?.supported ? (
              <>
                <VersionPicker
                  versions={versionsQuery.data.versions}
                  value={version}
                  onChange={setVersion}
                />
                <p className="text-xs text-muted-foreground">
                  Only branches, tags or commit SHAs that exist in the app’s
                  source repository can be seeded.
                </p>
              </>
            ) : (
              <Input
                id="seed-version"
                value={version}
                onChange={(e) => setVersion(e.target.value)}
                placeholder="e.g. 1.4.2 or a commit SHA"
                autoComplete="off"
                className="font-mono"
              />
            )}
          </div>

          {replaces && (
            <p className="flex items-start gap-2 rounded-md bg-status-warning/10 p-2.5 text-xs">
              <AlertTriangle
                aria-hidden
                className="mt-0.5 size-3.5 shrink-0 text-status-warning"
              />
              <span>
                <span className="font-mono">{replaces}</span> is currently
                deployed in {ring}; it becomes the previous version (available
                for rollback).
              </span>
            </p>
          )}

          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={!ring || !version.trim() || seed.isPending}
            >
              {seed.isPending && (
                <Loader2 aria-hidden className="size-4 animate-spin" />
              )}
              Seed {ring || "…"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * Searchable picker over the branches/tags that actually exist in the app's
 * source repository. Anything typed that isn't in the list can still be used
 * as a commit SHA — the server verifies it against the repo before deploying.
 */
function VersionPicker({
  versions,
  value,
  onChange,
}: {
  versions: AppVersion[];
  value: string;
  onChange: (v: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState("");

  const branches = versions.filter((v) => v.type === "branch");
  const tags = versions.filter((v) => v.type === "tag");
  const query = search.trim();
  const exactMatch = versions.some((v) => v.name === query);

  const pick = (name: string) => {
    onChange(name);
    setOpen(false);
    setSearch("");
  };

  const item = (v: AppVersion) => (
    <CommandItem key={`${v.type}-${v.name}`} value={v.name} onSelect={pick}>
      {v.type === "branch" ? (
        <GitBranch aria-hidden className="size-4" />
      ) : (
        <Tag aria-hidden className="size-4" />
      )}
      <span className="truncate font-mono text-xs">{v.name}</span>
      <Check
        aria-hidden
        className={cn(
          "ml-auto size-4",
          value === v.name ? "opacity-100" : "opacity-0",
        )}
      />
    </CommandItem>
  );

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>
        <Button
          type="button"
          variant="outline"
          role="combobox"
          aria-expanded={open}
          id="seed-version"
          className="w-full justify-between font-normal"
        >
          {value ? (
            <span className="truncate font-mono">{value}</span>
          ) : (
            <span className="text-muted-foreground">
              Select a branch, tag or SHA…
            </span>
          )}
          <ChevronsUpDown aria-hidden className="size-4 shrink-0 opacity-50" />
        </Button>
      </PopoverTrigger>
      <PopoverContent
        className="w-[var(--radix-popover-trigger-width)] p-0"
        align="start"
      >
        <Command>
          <CommandInput
            placeholder="Search, or paste a commit SHA…"
            value={search}
            onValueChange={setSearch}
          />
          <CommandList>
            <CommandEmpty>No matching branch or tag.</CommandEmpty>
            {branches.length > 0 && (
              <CommandGroup heading="Branches">
                {branches.map(item)}
              </CommandGroup>
            )}
            {tags.length > 0 && (
              <CommandGroup heading="Tags">{tags.map(item)}</CommandGroup>
            )}
            {query && !exactMatch && (
              <CommandGroup heading="Commit SHA">
                <CommandItem value={query} onSelect={() => pick(query)}>
                  <Hash aria-hidden className="size-4" />
                  <span className="truncate font-mono text-xs">{query}</span>
                  <span className="ml-auto text-xs text-muted-foreground">
                    verified on submit
                  </span>
                </CommandItem>
              </CommandGroup>
            )}
          </CommandList>
        </Command>
      </PopoverContent>
    </Popover>
  );
}

function PromoteDialog({
  app,
  rings,
  fromRing,
  onClose,
}: {
  app: string;
  rings: RingView[];
  fromRing: string | null;
  onClose: () => void;
}) {
  const promote = usePromoteMutation(app);
  const i = rings.findIndex((r) => r.ring.name === fromRing);
  const source = i >= 0 ? rings[i] : undefined;
  const target = i >= 0 ? rings[i + 1] : undefined;
  const toProd = target?.ring.name === "prod";

  return (
    <AlertDialog open={!!fromRing} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>
            Promote to {target?.ring.label ?? "the next ring"}?
          </AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              <p>
                <span className="font-mono font-medium">
                  {source?.current_version}
                </span>{" "}
                moves from <span className="font-medium">{fromRing}</span> to{" "}
                <span className="font-medium">{target?.ring.name}</span> for{" "}
                <span className="font-medium">{app}</span>.
              </p>
              <p>
                The source ring is health-checked first. After deploying, the
                target is health-checked with retries and rolled back
                automatically if it stays unhealthy.
              </p>
              {toProd && (
                <p className="flex items-start gap-2 rounded-md bg-status-critical/10 p-2.5 text-status-critical">
                  <AlertTriangle
                    aria-hidden
                    className="mt-0.5 size-4 shrink-0"
                  />
                  This deploys to Production.
                </p>
              )}
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            onClick={() =>
              fromRing && promote.mutate({ fromRing }, { onSuccess: onClose })
            }
          >
            Promote
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}

function RollbackDialog({
  app,
  rings,
  ring,
  onClose,
}: {
  app: string;
  rings: RingView[];
  ring: string | null;
  onClose: () => void;
}) {
  const rollback = useRollbackMutation(app);
  const view = rings.find((r) => r.ring.name === ring);

  return (
    <AlertDialog open={!!ring} onOpenChange={(o) => !o && onClose()}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Roll back {ring}?</AlertDialogTitle>
          <AlertDialogDescription asChild>
            <div className="space-y-2">
              <p>
                <span className="font-medium">{app}</span> in{" "}
                <span className="font-medium">{ring}</span> returns from{" "}
                <span className="font-mono font-medium">
                  {view?.current_version || "—"}
                </span>{" "}
                to{" "}
                <span className="font-mono font-medium">
                  {view?.previous_version}
                </span>
                .
              </p>
              <p>
                The current version becomes the new “previous”, so you can roll
                forward again if needed.
              </p>
            </div>
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            className="bg-status-critical text-white hover:bg-status-critical/90"
            onClick={() =>
              ring && rollback.mutate({ ring }, { onSuccess: onClose })
            }
          >
            Roll back
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
