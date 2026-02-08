import SwiftUI
import WebKit

struct ContentView: View {
    @ObservedObject var backend: BackendManager
    @StateObject private var notifications: NotificationManager

    init(backend: BackendManager) {
        self._backend = ObservedObject(wrappedValue: backend)
        self._notifications = StateObject(wrappedValue: NotificationManager(baseURL: backend.baseURL))
    }

    var body: some View {
        ZStack {
            switch backend.state {
            case .stopped, .starting:
                LaunchView(backend: backend)
            case .needsPairing:
                PairingView(backend: backend)
            case .running:
                WebViewContainer(url: backend.baseURL)
            case .error(let message):
                ErrorView(message: message, backend: backend)
            }
        }
        .background(Color(nsColor: .windowBackgroundColor))
        .onAppear {
            backend.start()
        }
        .onChange(of: backend.state) { _, newState in
            if newState == .running {
                notifications.start()
            } else {
                notifications.stop()
            }
        }
    }
}

// MARK: - Launch screen

struct LaunchView: View {
    @ObservedObject var backend: BackendManager

    var body: some View {
        VStack(spacing: 20) {
            ProgressView()
                .scaleEffect(1.5)
            Text("Starting OpenMessage...")
                .font(.title3)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

// MARK: - Error screen

struct ErrorView: View {
    let message: String
    @ObservedObject var backend: BackendManager

    var body: some View {
        VStack(spacing: 16) {
            Image(systemName: "exclamationmark.triangle")
                .font(.system(size: 48))
                .foregroundStyle(.orange)
            Text("Something went wrong")
                .font(.title2)
            Text(message)
                .font(.body)
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 40)
            Button("Try again") {
                backend.stop()
                backend.start()
            }
            .buttonStyle(.borderedProminent)
            .controlSize(.large)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

// MARK: - WebView wrapper

struct WebViewContainer: NSViewRepresentable {
    let url: URL

    func makeNSView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.preferences.setValue(true, forKey: "developerExtrasEnabled")

        let webView = WKWebView(frame: .zero, configuration: config)
        webView.setValue(false, forKey: "drawsBackground") // Transparent during load
        webView.load(URLRequest(url: url))
        return webView
    }

    func updateNSView(_ webView: WKWebView, context: Context) {
        // Only reload if URL changed
        if webView.url != url {
            webView.load(URLRequest(url: url))
        }
    }
}
