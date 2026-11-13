package io.github.chicohaager.moviescast

import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import androidx.activity.compose.BackHandler
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.activity.result.contract.ActivityResultContracts
import androidx.activity.viewModels
import androidx.appcompat.app.AppCompatActivity
import androidx.compose.runtime.getValue
import androidx.core.content.ContextCompat
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import io.github.chicohaager.moviescast.ui.DetailScreen
import io.github.chicohaager.moviescast.ui.LibraryScreen
import io.github.chicohaager.moviescast.ui.LoginScreen
import io.github.chicohaager.moviescast.ui.MoviesCastTheme
import io.github.chicohaager.moviescast.core.MediaItem

class MainActivity : AppCompatActivity() {
    private val model: AppViewModel by viewModels { AppViewModel.Factory }
    private var leftForPlayer = false

    /** What to do once the home network may be reached; null when nothing waits. */
    private var afterPermission: (() -> Unit)? = null

    private val askLocalNetwork = registerForActivityResult(ActivityResultContracts.RequestPermission()) { granted ->
        val next = afterPermission
        afterPermission = null
        if (granted) next?.invoke() else model.localNetworkDenied()
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            MoviesCastTheme {
                val state by model.state.collectAsStateWithLifecycle()
                BackHandler(enabled = state.stack.size > 1) { model.back() }
                when (state.screen) {
                    Screen.Login -> LoginScreen(state, model.lastServer, model.lastUser, ::login, ::openAppSettings)
                    Screen.Library -> LibraryScreen(state, model::search, model::open, ::refresh, model::logout, ::openAppSettings)
                    is Screen.Detail -> DetailScreen(state, { model.back() }, model::open, ::play)
                }
            }
        }
        // A saved login reads the library at once; on Android 17 that needs the permission first.
        if (savedInstanceState == null && !localNetworkAllowed()) withLocalNetwork { model.refresh() }
    }

    override fun onResume() {
        super.onResume()
        // The resume point changed while the player was open.
        if (leftForPlayer) {
            leftForPlayer = false
            model.refresh()
        }
    }

    private fun login(server: String, user: String, password: String) = withLocalNetwork { model.login(server, user, password) }

    private fun refresh() = withLocalNetwork { model.refresh() }

    /**
     * Android 17 (API 37) asks the person before an app may reach addresses in the home network
     * (ACCESS_LOCAL_NETWORK, a runtime permission). Earlier versions have no such permission.
     */
    private fun localNetworkAllowed(): Boolean =
        Build.VERSION.SDK_INT < LOCAL_NETWORK_SDK ||
            ContextCompat.checkSelfPermission(this, LOCAL_NETWORK) == PackageManager.PERMISSION_GRANTED

    private fun withLocalNetwork(action: () -> Unit) {
        if (localNetworkAllowed()) {
            action()
            return
        }
        afterPermission = action
        askLocalNetwork.launch(LOCAL_NETWORK)
    }

    private fun openAppSettings() {
        startActivity(Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS, Uri.fromParts("package", packageName, null)))
    }

    private fun play(item: MediaItem, fromSeconds: Long) {
        leftForPlayer = true
        startActivity(PlayerActivity.intent(this, item, fromSeconds))
    }

    private companion object {
        const val LOCAL_NETWORK = "android.permission.ACCESS_LOCAL_NETWORK"
        const val LOCAL_NETWORK_SDK = 37
    }
}
