"use client";

import { useState } from "react";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { usePrefsStore } from "@/lib/stores";
import type { AppGroup } from "@/lib/types";
import { cn } from "@/lib/utils";

/** Create or edit an application group: name + membership checklist. */
export function GroupDialog({
  open,
  onOpenChange,
  group,
  apps,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  group?: AppGroup;
  apps: string[];
}) {
  const createGroup = usePrefsStore((s) => s.createGroup);
  const updateGroup = usePrefsStore((s) => s.updateGroup);
  // The dialog is mounted fresh on every open (see Sidebar), so plain initial
  // state is enough — no reset effect needed.
  const [name, setName] = useState(group?.name ?? "");
  const [selected, setSelected] = useState<string[]>(group?.apps ?? []);

  const toggle = (app: string) =>
    setSelected((s) =>
      s.includes(app) ? s.filter((a) => a !== app) : [...s, app],
    );

  const save = () => {
    const trimmed = name.trim();
    if (!trimmed) return;
    if (group) {
      updateGroup(group.id, trimmed, selected);
    } else {
      createGroup(trimmed, selected);
    }
    onOpenChange(false);
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{group ? "Edit group" : "New group"}</DialogTitle>
          <DialogDescription>
            Groups are personal — they live in this browser only.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="group-name">Name</Label>
            <Input
              id="group-name"
              value={name}
              placeholder="e.g. Payments team"
              onChange={(e) => setName(e.target.value)}
              autoFocus
            />
          </div>
          <div className="space-y-2">
            <Label>Applications</Label>
            <div className="max-h-56 space-y-1 overflow-y-auto rounded-md border p-2">
              {apps.length === 0 && (
                <p className="px-1 text-sm text-muted-foreground">
                  No applications available.
                </p>
              )}
              {apps.map((app) => {
                const checked = selected.includes(app);
                return (
                  <button
                    key={app}
                    type="button"
                    onClick={() => toggle(app)}
                    className={cn(
                      "flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-left text-sm hover:bg-accent",
                      checked && "bg-accent/60",
                    )}
                  >
                    <span
                      className={cn(
                        "flex size-4 items-center justify-center rounded border text-[10px]",
                        checked
                          ? "border-primary bg-primary text-primary-foreground"
                          : "border-input",
                      )}
                      aria-hidden
                    >
                      {checked ? "✓" : ""}
                    </span>
                    <span className="truncate">{app}</span>
                  </button>
                );
              })}
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button onClick={save} disabled={!name.trim()}>
            {group ? "Save changes" : "Create group"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
