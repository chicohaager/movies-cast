package io.github.chicohaager.moviescast

import android.app.Application
import android.os.Build
import coil3.ImageLoader
import coil3.PlatformContext
import coil3.SingletonImageLoader
import coil3.network.okhttp.OkHttpNetworkFetcherFactory
import io.github.chicohaager.moviescast.core.Device
import io.github.chicohaager.moviescast.core.MoviesClient
import io.github.chicohaager.moviescast.core.ServerSearch
import io.github.chicohaager.moviescast.core.Session
import java.util.concurrent.TimeUnit
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.async
import kotlinx.coroutines.coroutineScope
import okhttp3.HttpUrl.Companion.toHttpUrlOrNull
import okhttp3.Interceptor
import okhttp3.OkHttpClient
import okhttp3.Response

/** The few objects every screen shares; built once, by hand. */
open class MoviesCastApp : Application(), SingletonImageLoader.Factory {

    /** The preferences file of the login; the tests on a device use one of their own, so they never touch a real login. */
    protected open val storeName: String get() = "session"

    lateinit var store: SessionStore
        private set
    lateinit var client: MoviesClient
        private set

    /** For posters and the film itself: adds the login header, but only towards the server logged in to. */
    lateinit var mediaHttp: OkHttpClient
        private set

    /** Outlives a screen: the "stopped at" report must still go out when the player closes. */
    val appScope = CoroutineScope(SupervisorJob() + Dispatchers.Main.immediate)

    /** The current login, or null. Read by the interceptor on network threads. */
    @Volatile
    var session: Session? = null
        private set

    override fun onCreate() {
        super.onCreate()
        store = SessionStore(this, storeName)
        val http = OkHttpClient.Builder()
            .connectTimeout(8, TimeUnit.SECONDS)
            .readTimeout(20, TimeUnit.SECONDS)
            .build()
        client = MoviesClient(http, Device(store.deviceId, Build.MODEL ?: "Android", BuildConfig.VERSION_NAME))
        mediaHttp = http.newBuilder()
            .followRedirects(false)
            .followSslRedirects(false)
            .addInterceptor(AuthInterceptor())
            .build()
        session = store.load()
    }

    /**
     * What announces itself in the home network: one list of base URLs per server, to be asked
     * with [MoviesClient.confirm]. Jellyfin servers by their search call, ZimaOS boxes by mDNS;
     * both run at the same time. The tests on a device put their fake server here.
     */
    open suspend fun announced(): List<List<String>> = coroutineScope {
        val jellyfin = async { ServerSearch.askJellyfin(ServerSearch.broadcastTargets(), SEARCH_MS) }
        val zima = async { ZimaSearch(this@MoviesCastApp).boxes(SEARCH_MS).map { listOf(ServerSearch.zimaCandidate(it.first, it.second)) } }
        val out = jellyfin.await() + zima.await()
        AppLog.i("search: ${jellyfin.await().size} answer(s) to Jellyfin's call, ${zima.await().size} ZimaOS box(es)")
        out
    }

    fun setSession(value: Session?) {
        session = value
        if (value == null) store.clear() else store.save(value)
    }

    override fun newImageLoader(context: PlatformContext): ImageLoader =
        ImageLoader.Builder(context)
            .components { add(OkHttpNetworkFetcherFactory(callFactory = { mediaHttp })) }
            .build()

    private companion object {
        const val SEARCH_MS = 4000L
    }

    private inner class AuthInterceptor : Interceptor {
        override fun intercept(chain: Interceptor.Chain): Response {
            val request = chain.request()
            val s = session
            val base = s?.baseUrl?.toHttpUrlOrNull()
            val url = request.url
            val sameServer = base != null && url.scheme == base.scheme && url.host == base.host && url.port == base.port
            if (s == null || !sameServer) return chain.proceed(request)
            return chain.proceed(request.newBuilder().header("X-Emby-Authorization", client.authHeader(s.token)).build())
        }
    }
}
