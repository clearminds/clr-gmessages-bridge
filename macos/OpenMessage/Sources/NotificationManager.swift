import Foundation
import UserNotifications
import os

/// Polls the backend for new messages and sends native macOS notifications.
@MainActor
final class NotificationManager: ObservableObject {
    private let logger = Logger(subsystem: "com.openmessage.app", category: "Notifications")
    private var pollTask: Task<Void, Never>?
    private var lastSeenTimestamps: [String: Int64] = [:] // conversationID → latest timestamp
    private var baseURL: URL
    private var isFirstLoad = true

    init(baseURL: URL) {
        self.baseURL = baseURL
    }

    func start() {
        requestPermission()
        pollTask = Task {
            // Initial delay to let backend populate
            try? await Task.sleep(for: .seconds(5))
            while !Task.isCancelled {
                await checkForNewMessages()
                try? await Task.sleep(for: .seconds(3))
            }
        }
    }

    func stop() {
        pollTask?.cancel()
        pollTask = nil
    }

    private func requestPermission() {
        UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge]) { granted, error in
            if let error {
                self.logger.error("Notification permission error: \(error)")
            }
            self.logger.info("Notification permission granted: \(granted)")
        }
    }

    private func checkForNewMessages() async {
        do {
            // Fetch conversations
            let url = baseURL.appendingPathComponent("api/conversations")
            let (data, _) = try await URLSession.shared.data(from: url.appending(queryItems: [
                URLQueryItem(name: "limit", value: "20")
            ]))
            guard let convos = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return }

            for convo in convos {
                guard let convID = convo["ConversationID"] as? String,
                      let lastTS = convo["LastMessageTS"] as? Int64,
                      let name = convo["Name"] as? String else { continue }

                let previousTS = lastSeenTimestamps[convID] ?? 0

                if isFirstLoad {
                    // On first load, just record timestamps without notifying
                    lastSeenTimestamps[convID] = lastTS
                    continue
                }

                if lastTS > previousTS && previousTS > 0 {
                    // New message in this conversation — fetch it
                    await fetchAndNotify(conversationID: convID, name: name, since: previousTS)
                }

                lastSeenTimestamps[convID] = lastTS
            }

            isFirstLoad = false
        } catch {
            logger.debug("Poll error: \(error)")
        }
    }

    private func fetchAndNotify(conversationID: String, name: String, since: Int64) async {
        do {
            let url = baseURL.appendingPathComponent("api/conversations/\(conversationID)/messages")
            let (data, _) = try await URLSession.shared.data(from: url.appending(queryItems: [
                URLQueryItem(name: "limit", value: "5")
            ]))
            guard let msgs = try? JSONSerialization.jsonObject(with: data) as? [[String: Any]] else { return }

            // Find new incoming messages (not from me)
            for msg in msgs {
                guard let ts = msg["TimestampMS"] as? Int64,
                      ts > since,
                      let isFromMe = msg["IsFromMe"] as? Bool,
                      !isFromMe else { continue }

                let body = msg["Body"] as? String ?? "New message"
                let sender = msg["SenderName"] as? String ?? name

                sendNotification(title: sender, body: body, conversationID: conversationID)
                break // One notification per conversation per poll
            }
        } catch {
            logger.debug("Fetch messages error: \(error)")
        }
    }

    private func sendNotification(title: String, body: String, conversationID: String) {
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        content.userInfo = ["conversationID": conversationID]

        let request = UNNotificationRequest(
            identifier: "msg-\(conversationID)-\(Date().timeIntervalSince1970)",
            content: content,
            trigger: nil // Deliver immediately
        )

        UNUserNotificationCenter.current().add(request) { error in
            if let error {
                self.logger.error("Failed to send notification: \(error)")
            }
        }
    }
}
