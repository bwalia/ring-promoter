"use client";

import { useState } from "react";
import {
  ChevronRight,
  FolderPlus,
  Layers,
  MoreHorizontal,
  Pencil,
  Search,
  Star,
  Trash2,
} from "lucide-react";
import { GroupDialog } from "@/components/group-dialog";
import { RingMark } from "@/components/ring-mark";
import { Button } from "@/components/ui/button";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { Skeleton } from "@/components/ui/skeleton";
import {
  useApps,
  useAppTitle,
  useDeleteGroup,
  useGroups,
  useUpdateGroup,
} from "@/lib/queries";
import { usePrefsStore } from "@/lib/stores";
import type { AppGroup } from "@/lib/types";
import { cn } from "@/lib/utils";

export function Sidebar({
  onNavigate,
  searchInputId,
}: {
  onNavigate?: () => void;
  /** Only the desktop instance gets the id targeted by the "/" shortcut —
      the sidebar also renders inside the mobile sheet, and ids must be unique. */
  searchInputId?: string;
}) {
  const { data, isPending, error, refetch } = useApps();
  const apps = data?.apps ?? [];
  const [filter, setFilter] = useState("");
  const [groupDialog, setGroupDialog] = useState<
    { open: boolean; group?: AppGroup }
  >({ open: false });

  const favorites = usePrefsStore((s) => s.favorites);
  const groups = useGroups().data ?? [];

  const q = filter.trim().toLowerCase();
  const title = useAppTitle();
  const match = (a: string) =>
    a.toLowerCase().includes(q) || title(a).toLowerCase().includes(q);
  const known = (a: string) => apps.includes(a);

  const favApps = favorites.filter(known).filter(match);
  const allApps = apps.filter(match);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center gap-2 px-4 pb-2 pt-4">
        <div className="flex size-7 items-center justify-center rounded-md bg-primary text-primary-foreground">
          <RingMark className="size-[1.15rem]" />
        </div>
        <span className="font-display text-sm font-bold tracking-tight">Ring Promoter</span>
      </div>

      <div className="px-3 pb-2">
        <div className="relative">
          <Search
            aria-hidden
            className="absolute left-2.5 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
          />
          <Input
            id={searchInputId}
            placeholder="Filter applications…"
            className="h-8 pl-8 text-sm"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            aria-label="Filter applications"
          />
          <kbd className="pointer-events-none absolute right-2.5 top-1/2 -translate-y-1/2 rounded border bg-muted px-1 font-mono text-[10px] text-muted-foreground">
            /
          </kbd>
        </div>
      </div>

      <ScrollArea className="min-h-0 flex-1 px-3">
        <div className="space-y-4 pb-6 pt-1">
          {isPending && (
            <div className="space-y-2 px-1 pt-2">
              {Array.from({ length: 6 }).map((_, i) => (
                <Skeleton key={i} className="h-7 w-full" />
              ))}
            </div>
          )}

          {error && (
            <div className="px-1 pt-2 text-sm text-muted-foreground">
              <p className="text-status-critical">Failed to load apps.</p>
              <Button
                variant="link"
                size="sm"
                className="h-auto p-0"
                onClick={() => refetch()}
              >
                Retry
              </Button>
            </div>
          )}

          {!isPending && !error && apps.length === 0 && (
            <p className="px-1 pt-2 text-sm text-muted-foreground">
              No applications configured. Add one under <code>apps:</code> in
              the server config.
            </p>
          )}

          {favApps.length > 0 && (
            <Section id="favorites" label="Favorites">
              {favApps.map((a) => (
                <AppRow key={a} app={a} onNavigate={onNavigate} />
              ))}
            </Section>
          )}

          {groups.length > 0 && (
            <Section
              id="groups"
              label="Groups"
              action={
                <Button
                  variant="ghost"
                  size="icon"
                  className="size-6"
                  aria-label="New group"
                  onClick={() => setGroupDialog({ open: true })}
                >
                  <FolderPlus aria-hidden className="size-3.5" />
                </Button>
              }
            >
              {groups.map((g) => (
                <GroupSection
                  key={g.id}
                  group={g}
                  filterMatch={match}
                  known={known}
                  onEdit={() => setGroupDialog({ open: true, group: g })}
                  onNavigate={onNavigate}
                />
              ))}
            </Section>
          )}

          {apps.length > 0 && (
            <Section
              id="all"
              label="Applications"
              action={
                groups.length === 0 ? (
                  <Button
                    variant="ghost"
                    size="icon"
                    className="size-6"
                    aria-label="New group"
                    onClick={() => setGroupDialog({ open: true })}
                  >
                    <FolderPlus aria-hidden className="size-3.5" />
                  </Button>
                ) : undefined
              }
            >
              {allApps.length === 0 ? (
                <p className="px-2 py-1 text-xs text-muted-foreground">
                  No matches for “{filter}”.
                </p>
              ) : (
                allApps.map((a) => (
                  <AppRow key={a} app={a} onNavigate={onNavigate} />
                ))
              )}
            </Section>
          )}
        </div>
      </ScrollArea>

      {groupDialog.open && (
        <GroupDialog
          open
          group={groupDialog.group}
          apps={apps}
          onOpenChange={(open) => setGroupDialog((s) => ({ ...s, open }))}
        />
      )}
    </div>
  );
}

function Section({
  id,
  label,
  action,
  children,
}: {
  id: string;
  label: string;
  action?: React.ReactNode;
  children: React.ReactNode;
}) {
  const collapsed = usePrefsStore((s) => !!s.collapsed[id]);
  const toggleCollapsed = usePrefsStore((s) => s.toggleCollapsed);

  return (
    <Collapsible open={!collapsed} onOpenChange={() => toggleCollapsed(id)}>
      <div className="flex items-center justify-between">
        <CollapsibleTrigger className="group flex flex-1 items-center gap-1 rounded-md px-1 py-1 text-xs font-medium text-muted-foreground hover:text-foreground">
          <ChevronRight
            aria-hidden
            className={cn(
              "size-3 transition-transform",
              !collapsed && "rotate-90",
            )}
          />
          {label}
        </CollapsibleTrigger>
        {action}
      </div>
      <CollapsibleContent className="mt-1 space-y-0.5">
        {children}
      </CollapsibleContent>
    </Collapsible>
  );
}

function GroupSection({
  group,
  filterMatch,
  known,
  onEdit,
  onNavigate,
}: {
  group: AppGroup;
  filterMatch: (a: string) => boolean;
  known: (a: string) => boolean;
  onEdit: () => void;
  onNavigate?: () => void;
}) {
  const collapsed = usePrefsStore((s) => !!s.collapsed[`group:${group.id}`]);
  const toggleCollapsed = usePrefsStore((s) => s.toggleCollapsed);
  const deleteGroup = useDeleteGroup();
  const selectGroup = usePrefsStore((s) => s.selectGroup);
  const active = usePrefsStore((s) => s.selectedGroup === group.id);
  const members = group.apps.filter(known).filter(filterMatch);

  return (
    <Collapsible
      open={!collapsed}
      onOpenChange={() => toggleCollapsed(`group:${group.id}`)}
    >
      <div
        className={cn(
          "group/g flex items-center justify-between rounded-md pr-1",
          active
            ? "bg-sidebar-accent text-sidebar-accent-foreground"
            : "hover:bg-sidebar-accent",
        )}
      >
        <CollapsibleTrigger
          className="py-1.5 pl-2 pr-0.5"
          aria-label={`${collapsed ? "Expand" : "Collapse"} ${group.name}`}
        >
          <ChevronRight
            aria-hidden
            className={cn(
              "size-3.5 text-muted-foreground transition-transform",
              !collapsed && "rotate-90",
            )}
          />
        </CollapsibleTrigger>
        {/* The group name opens the group page (the chevron collapses). */}
        <button
          type="button"
          className="flex min-w-0 flex-1 items-center gap-1.5 py-1.5 pr-1 text-left text-sm"
          onClick={() => {
            selectGroup(group.id);
            onNavigate?.();
          }}
        >
          <span className="truncate">{group.name}</span>
          <span className="text-xs text-muted-foreground">
            {group.apps.length}
          </span>
        </button>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              variant="ghost"
              size="icon"
              className="size-6 opacity-0 group-hover/g:opacity-100 data-[state=open]:opacity-100"
              aria-label={`Group ${group.name} options`}
            >
              <MoreHorizontal aria-hidden className="size-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="start">
            <DropdownMenuItem onClick={onEdit}>
              <Pencil aria-hidden className="size-4" /> Edit group
            </DropdownMenuItem>
            <DropdownMenuItem
              variant="destructive"
              onClick={() => deleteGroup.mutate(group.id)}
            >
              <Trash2 aria-hidden className="size-4" /> Delete group
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
      <CollapsibleContent className="ml-3 space-y-0.5 border-l pl-1">
        {members.length === 0 ? (
          <p className="px-2 py-1 text-xs text-muted-foreground">Empty</p>
        ) : (
          members.map((a) => <AppRow key={a} app={a} onNavigate={onNavigate} />)
        )}
      </CollapsibleContent>
    </Collapsible>
  );
}

function AppRow({ app, onNavigate }: { app: string; onNavigate?: () => void }) {
  const title = useAppTitle();
  const selectedApp = usePrefsStore((s) => s.selectedApp);
  const groupActive = usePrefsStore((s) => !!s.selectedGroup);
  const selectApp = usePrefsStore((s) => s.selectApp);
  const favorites = usePrefsStore((s) => s.favorites);
  const toggleFavorite = usePrefsStore((s) => s.toggleFavorite);
  const groups = useGroups().data ?? [];
  const updateGroup = useUpdateGroup();
  // A group page showing means no app row is the active view, even though
  // selectedApp is retained behind the scenes.
  const active = !groupActive && selectedApp === app;
  const favorite = favorites.includes(app);

  return (
    <div
      className={cn(
        "group/row flex items-center rounded-md",
        active
          ? "bg-sidebar-accent text-sidebar-accent-foreground"
          : "hover:bg-sidebar-accent/60",
      )}
    >
      <button
        type="button"
        className={cn(
          "flex min-w-0 flex-1 items-center gap-2 px-2 py-1.5 text-left text-sm",
          !active && "text-muted-foreground hover:text-foreground",
        )}
        onClick={() => {
          selectApp(app);
          onNavigate?.();
        }}
      >
        <span className="truncate">{title(app)}</span>
        {favorite && (
          <Star
            aria-hidden
            className="size-3 shrink-0 fill-status-warning text-status-warning"
          />
        )}
      </button>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="ghost"
            size="icon"
            className="mr-1 size-6 shrink-0 opacity-0 group-hover/row:opacity-100 data-[state=open]:opacity-100"
            aria-label={`${title(app)} options`}
          >
            <MoreHorizontal aria-hidden className="size-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start">
          <DropdownMenuItem onClick={() => toggleFavorite(app)}>
            <Star aria-hidden className="size-4" />
            {favorite ? "Remove favorite" : "Add to favorites"}
          </DropdownMenuItem>
          {groups.length > 0 && (
            <>
              <DropdownMenuSeparator />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger>
                  <Layers aria-hidden className="mr-2 size-4" /> Groups
                </DropdownMenuSubTrigger>
                <DropdownMenuSubContent>
                  <DropdownMenuLabel>Add to group</DropdownMenuLabel>
                  {groups.map((g) => {
                    const inGroup = g.apps.includes(app);
                    return (
                      <DropdownMenuCheckboxItem
                        key={g.id}
                        checked={inGroup}
                        onCheckedChange={() =>
                          updateGroup.mutate({
                            id: g.id,
                            name: g.name,
                            apps: inGroup
                              ? g.apps.filter((a) => a !== app)
                              : [...g.apps, app],
                          })
                        }
                      >
                        {g.name}
                      </DropdownMenuCheckboxItem>
                    );
                  })}
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            </>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}
