import SwiftUI

/// First run, and the "add another control plane" screen.
struct OnboardingView: View {
    @Environment(AppSession.self) private var session
    @State private var isAddingInstance = false

    var body: some View {
        NavigationStack {
            List {
                if !session.instances.isEmpty {
                    Section("Saved control planes") {
                        ForEach(session.instances.instances) { instance in
                            Button {
                                if !session.activate(instance) { isAddingInstance = true }
                            } label: {
                                InstanceRow(instance: instance)
                            }
                            .buttonStyle(.plain)
                        }
                        .onDelete { offsets in
                            for index in offsets {
                                session.instances.remove(session.instances.instances[index])
                            }
                        }
                    }
                }

                Section {
                    Button {
                        isAddingInstance = true
                    } label: {
                        Label("Connect to a control plane", systemImage: "link")
                    }
                    Button {
                        session.enterDemoMode()
                    } label: {
                        Label("Explore in demo mode", systemImage: "theatermasks")
                    }
                } footer: {
                    Text(
                        "Demo mode runs the whole app from bundled sample data — no server, "
                            + "no network, nothing deployed."
                    )
                }
            }
            .navigationTitle("Ring Promoter")
            .overlay {
                if session.instances.isEmpty {
                    WelcomePanel { isAddingInstance = true } onDemo: {
                        session.enterDemoMode()
                    }
                }
            }
            .sheet(isPresented: $isAddingInstance) {
                AddInstanceView()
            }
        }
    }
}

/// The empty state for a fresh install.
private struct WelcomePanel: View {
    let onConnect: () -> Void
    let onDemo: () -> Void

    var body: some View {
        VStack(spacing: 14) {
            Image(systemName: "square.stack.3d.up.fill")
                .font(.system(size: 48, weight: .light))
                .foregroundStyle(.tint)
            Text("Promote with confidence")
                .font(.title3.weight(.semibold))
            Text(
                "Watch every application move through int → test → acc → prod, "
                    + "and act safely from your phone."
            )
            .font(.subheadline)
            .foregroundStyle(.secondary)
            .multilineTextAlignment(.center)

            VStack(spacing: 10) {
                Button(action: onConnect) {
                    Label("Connect to a control plane", systemImage: "link")
                        .frame(maxWidth: 300)
                }
                .buttonStyle(.borderedProminent)
                Button(action: onDemo) {
                    Label("Explore in demo mode", systemImage: "theatermasks")
                        .frame(maxWidth: 300)
                }
                .buttonStyle(.bordered)
            }
            .padding(.top, 6)
        }
        .padding(30)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(.background)
    }
}

/// One saved control plane in a list.
struct InstanceRow: View {
    let instance: Instance
    var isActive: Bool = false

    var body: some View {
        HStack(spacing: 12) {
            Circle()
                .fill(instance.tint.color)
                .frame(width: 12, height: 12)
                .overlay(Circle().strokeBorder(.primary.opacity(0.2)))
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(instance.name).font(.body.weight(.medium))
                    if isActive {
                        StatusBadge(text: "Active", systemImage: "checkmark", tint: .rpHealthy)
                    }
                }
                Text(instance.displayHost)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Spacer()
            if instance.isInsecure {
                Image(systemName: "lock.open.trianglebadge.exclamationmark")
                    .foregroundStyle(Color.rpGate)
                    .accessibilityLabel("Not using HTTPS")
            }
        }
        .contentShape(.rect)
        .accessibilityElement(children: .combine)
    }
}

#Preview("Onboarding") {
    PreviewHost { OnboardingView() }
}
