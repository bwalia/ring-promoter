import SwiftUI

/// The instance banner as a menu: tap it on any top-level screen to jump to
/// another saved control plane or add a new one, without a detour through
/// Settings.
///
/// The banner itself is unchanged, so "which cluster am I on?" still reads at
/// a glance; the menu only adds a way off it.
struct InstanceSwitcher: View {
    @Environment(AppSession.self) private var session
    @State private var isAddingInstance = false
    @State private var isManaging = false
    /// A saved instance whose token has gone from the Keychain; re-entering it
    /// is the only way to switch to it.
    @State private var needsToken: Instance?

    var body: some View {
        Menu {
            Section("Clusters") {
                ForEach(session.instances.instances) { instance in
                    Button {
                        switchTo(instance)
                    } label: {
                        if instance.id == session.activeInstance?.id {
                            Label(instance.name, systemImage: "checkmark")
                        } else {
                            Text(instance.name)
                        }
                        Text(instance.displayHost)
                    }
                }
            }
            Button {
                isAddingInstance = true
            } label: {
                Label("Add a cluster", systemImage: "plus.circle")
            }
            Button {
                isManaging = true
            } label: {
                Label("Manage clusters…", systemImage: "server.rack")
            }
            if !session.isDemo {
                Button {
                    session.enterDemoMode()
                } label: {
                    Label("Switch to demo mode", systemImage: "theatermasks")
                }
            }
        } label: {
            InstanceBanner(
                name: session.instanceName, tint: session.instanceTint,
                isDemo: session.isDemo
            )
        }
        .accessibilityHint("Switch cluster")
        .sheet(isPresented: $isAddingInstance) { AddInstanceView() }
        .sheet(isPresented: $isManaging) { ControlPlanesView(isSheet: true) }
        .sheet(item: $needsToken) { AddInstanceView(editing: $0) }
    }

    private func switchTo(_ instance: Instance) {
        guard instance.id != session.activeInstance?.id else { return }
        if !session.activate(instance) { needsToken = instance }
    }
}

#Preview("Instance switcher") {
    PreviewHost {
        NavigationStack {
            Text("Applications")
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) { InstanceSwitcher() }
                }
        }
    }
}
