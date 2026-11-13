package io.github.chicohaager.moviescast

import android.content.Context
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import io.github.chicohaager.moviescast.core.ServerSearch
import kotlin.coroutines.resume
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.delay
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlinx.coroutines.withTimeoutOrNull

/**
 * ZimaOS boxes that announce themselves in the home network (mDNS, `_zimaos._tcp`), found with
 * Android's own service search. Movies does not answer Jellyfin's search call, so this is how a
 * box with Movies is found; whether Movies is installed on it, the info page then tells.
 */
class ZimaSearch(context: Context) {
    private val nsd = context.getSystemService(Context.NSD_SERVICE) as NsdManager

    /** Address and port of every box that announced itself within [waitMs]. */
    suspend fun boxes(waitMs: Long): List<Pair<String, Int>> {
        val announced = Channel<NsdServiceInfo>(Channel.UNLIMITED)
        val out = LinkedHashSet<Pair<String, Int>>()
        val begun = System.nanoTime()
        fun ms() = (System.nanoTime() - begun) / 1_000_000
        val listener = object : NsdManager.DiscoveryListener {
            override fun onServiceFound(info: NsdServiceInfo) {
                announced.trySend(info)
            }

            override fun onStartDiscoveryFailed(type: String, code: Int) {
                AppLog.w("search: the service search did not start (code $code)")
                announced.close()
            }

            override fun onStopDiscoveryFailed(type: String, code: Int) = Unit
            override fun onDiscoveryStarted(type: String) = Unit
            override fun onDiscoveryStopped(type: String) = Unit
            override fun onServiceLost(info: NsdServiceInfo) = Unit
        }
        try {
            nsd.discoverServices(ServerSearch.ZIMAOS_SERVICE, NsdManager.PROTOCOL_DNS_SD, listener)
        } catch (e: RuntimeException) {
            AppLog.w("search: the service search is not available (${e.javaClass.simpleName})")
            return emptyList()
        }
        try {
            // One at a time: Android before 12 refuses a second resolve while one runs.
            withTimeoutOrNull(waitMs) {
                for (info in announced) {
                    // Android at times names only an IPv6 address at first (Pixel 7, Android 17: 5 of 8 searches); asked again, the IPv4 one comes.
                    var box: Pair<String, Int>? = null
                    var asked = 0
                    while (box == null && asked < RESOLVE_TRIES) {
                        if (asked > 0) delay(RESOLVE_PAUSE_MS)
                        box = resolve(info)
                        asked++
                    }
                    AppLog.i("search: ${if (box == null) "no address for a box" else "address of a box"} after ${ms()} ms, asked $asked time(s)")
                    box?.let { out.add(it) }
                }
            }
        } finally {
            try {
                nsd.stopServiceDiscovery(listener)
            } catch (_: IllegalArgumentException) {
                // the search never started
            }
        }
        return out.toList()
    }

    private companion object {
        const val RESOLVE_TRIES = 6
        const val RESOLVE_PAUSE_MS = 250L
    }

    @Suppress("DEPRECATION") // resolveService and host: their successors exist from Android 14 only
    private suspend fun resolve(info: NsdServiceInfo): Pair<String, Int>? = suspendCancellableCoroutine { waiting ->
        val listener = object : NsdManager.ResolveListener {
            override fun onResolveFailed(info: NsdServiceInfo, code: Int) {
                AppLog.w("search: a box did not give its address (code $code)")
                if (waiting.isActive) waiting.resume(null)
            }

            override fun onServiceResolved(resolved: NsdServiceInfo) {
                val addresses = if (Build.VERSION.SDK_INT >= 34) resolved.hostAddresses else listOfNotNull(resolved.host)
                val address = ServerSearch.ipv4(addresses)
                if (address == null) AppLog.w("search: a box gave ${addresses.size} address(es), none of them IPv4")
                if (waiting.isActive) waiting.resume(address?.let { it to resolved.port })
            }
        }
        try {
            nsd.resolveService(info, listener)
        } catch (_: RuntimeException) {
            if (waiting.isActive) waiting.resume(null)
        }
    }
}
