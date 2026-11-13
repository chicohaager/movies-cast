package io.github.chicohaager.moviescast

import android.content.ActivityNotFoundException
import android.content.Intent
import android.content.res.Configuration
import android.media.MediaCodecList
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
import androidx.annotation.OptIn
import androidx.media3.common.util.UnstableApi
import androidx.media3.decoder.ffmpeg.FfmpegLibrary
import io.github.chicohaager.moviescast.ui.AboutScreen
import io.github.chicohaager.moviescast.ui.DetailScreen
import io.github.chicohaager.moviescast.ui.Languages
import io.github.chicohaager.moviescast.ui.LicenceScreen
import io.github.chicohaager.moviescast.ui.LicencesScreen
import java.util.Locale
import io.github.chicohaager.moviescast.ui.LibraryScreen
import io.github.chicohaager.moviescast.ui.LoginScreen
import io.github.chicohaager.moviescast.ui.MoviesCastTheme
import io.github.chicohaager.moviescast.core.MediaItem

@OptIn(UnstableApi::class)
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
                when (val screen = state.screen) {
                    Screen.Login -> LoginScreen(state, model.lastServer, model.lastUser, ::login, ::openAppSettings) { model.show(Screen.About) }
                    Screen.Library -> LibraryScreen(state, model::search, model::open, ::refresh, model::logout, ::openAppSettings) { model.show(Screen.About) }
                    is Screen.Detail -> DetailScreen(state, { model.back() }, model::open, ::play)
                    Screen.About -> AboutScreen({ model.back() }, { model.show(Screen.Licences) }, ::openWeb, ::diagnostics, ::share)
                    Screen.Licences -> LicencesScreen({ model.back() }) { model.show(Screen.Licence(it)) }
                    is Screen.Licence -> LicenceScreen(screen.id) { model.back() }
                }
            }
        }
        // A saved login reads the library at once; on Android 17 that needs the permission first.
        // Without a login nothing is read yet: the question comes with "Log in", where its reason
        // is on the screen (the first start of 0.1.0 asked before anything was shown, Pixel 7, 2026-11-13).
        val loggedIn = (application as MoviesCastApp).session != null
        if (savedInstanceState == null && loggedIn && !localNetworkAllowed()) withLocalNetwork { model.refresh() }
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

    /** Opens a web address; false where no app can (a TV without a browser). */
    private fun openWeb(address: String): Boolean =
        try {
            startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(address)))
            true
        } catch (_: ActivityNotFoundException) {
            false
        }

    /**
     * What "Share diagnostics" shows and hands out: versions, the device, its decoders and the
     * player's last log lines. No address, account or film; [LogBook] scrubs addresses in addition.
     */
    private fun diagnostics(): String {
        val tv = (resources.configuration.uiMode and Configuration.UI_MODE_TYPE_MASK) == Configuration.UI_MODE_TYPE_TELEVISION
        val facts = buildList {
            add("app" to "${BuildConfig.VERSION_NAME} (${BuildConfig.VERSION_CODE}) ${BuildConfig.BUILD_TYPE}")
            add("android" to "${Build.VERSION.RELEASE} (API ${Build.VERSION.SDK_INT})")
            add("device" to "${Build.MANUFACTURER} ${Build.MODEL}, ${if (tv) "TV" else "phone or tablet"}")
            add("language" to Languages.chosen().ifEmpty { "system (${Locale.getDefault().toLanguageTag()})" })
            add("local network" to if (Build.VERSION.SDK_INT < LOCAL_NETWORK_SDK) "no permission needed" else if (localNetworkAllowed()) "allowed" else "not allowed")
            add("ffmpeg" to if (FfmpegLibrary.isAvailable()) FfmpegLibrary.getVersion().orEmpty() else "not available")
            model.state.value.server?.let { add("server" to "${it.product} ${it.version}") }
        }
        val decoders = MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos
            .filter { !it.isEncoder }
            .map { "${it.name}: ${it.supportedTypes.joinToString(", ")}" }
        return LogBook.report(facts, AppLog.book.lines(), decoders)
    }

    private fun share(text: String) {
        val send = Intent(Intent.ACTION_SEND).setType("text/plain")
            .putExtra(Intent.EXTRA_SUBJECT, "Movies-Cast ${BuildConfig.VERSION_NAME} diagnostics")
            .putExtra(Intent.EXTRA_TEXT, text)
        startActivity(Intent.createChooser(send, null))
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
