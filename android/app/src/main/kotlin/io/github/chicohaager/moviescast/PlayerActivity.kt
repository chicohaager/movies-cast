package io.github.chicohaager.moviescast

import android.app.PictureInPictureParams
import android.content.pm.PackageManager
import android.content.res.Configuration
import android.media.MediaCodecList
import android.os.Build
import android.util.Rational
import android.content.Context
import android.content.Intent
import android.graphics.Color
import android.net.Uri
import android.os.Bundle
import android.os.SystemClock
import android.view.Gravity
import android.view.View
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.FrameLayout
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import androidx.annotation.OptIn
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.core.view.WindowInsetsControllerCompat
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.media3.common.AudioAttributes
import androidx.media3.common.C
import androidx.media3.common.Format
import androidx.media3.common.MediaItem as ExoMediaItem
import androidx.media3.common.MediaMetadata
import androidx.media3.common.MimeTypes
import androidx.media3.common.PlaybackException
import androidx.media3.common.Player
import androidx.media3.common.Tracks
import androidx.media3.common.TrackSelectionOverride
import androidx.media3.common.VideoSize
import androidx.media3.common.util.UnstableApi
import androidx.media3.datasource.okhttp.OkHttpDataSource
import androidx.media3.decoder.ffmpeg.FfmpegLibrary
import androidx.media3.exoplayer.DefaultRenderersFactory
import androidx.media3.exoplayer.ExoPlayer
import androidx.media3.exoplayer.analytics.AnalyticsListener
import androidx.media3.exoplayer.source.DefaultMediaSourceFactory
import androidx.media3.session.MediaSession
import androidx.media3.ui.PlayerView
import io.github.chicohaager.moviescast.core.AudioPick
import io.github.chicohaager.moviescast.core.PlayState
import io.github.chicohaager.moviescast.core.Session
import java.io.IOException
import java.util.Locale
import java.util.UUID
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch

/**
 * Plays the original file straight from the server (Range requests, no conversion on the box)
 * and tells the server where the viewer is, so "continue watching" is right on every device.
 */
@OptIn(UnstableApi::class)
class PlayerActivity : AppCompatActivity() {
    private lateinit var app: MoviesCastApp
    private lateinit var session: Session
    private lateinit var itemId: String
    private var startMs = 0L
    private var player: ExoPlayer? = null
    private var mediaSession: MediaSession? = null
    private lateinit var message: TextView
    private lateinit var playerView: PlayerView
    private var playSessionId = UUID.randomUUID().toString()
    private var reported = false
    private var converting = false
    private val openedAt = SystemClock.elapsedRealtime()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        app = application as MoviesCastApp
        val current = app.session
        @Suppress("DEPRECATION") // the typed variant exists from Android 13 only
        val queue = (intent.getSerializableExtra(EXTRA_QUEUE) as? ArrayList<*>)?.filterIsInstance<QueueEntry>().orEmpty()
        if (current == null || queue.isEmpty()) {
            finish()
            return
        }
        session = current

        val view = PlayerView(this).also { playerView = it }.apply {
            keepScreenOn = true
            setShowSubtitleButton(true)
            // An episode brings the ones after it; the buttons then step through them.
            setShowNextButton(queue.size > 1)
            setShowPreviousButton(queue.size > 1)
            controllerShowTimeoutMs = 3500
        }
        message = TextView(this).apply {
            setTextColor(Color.WHITE)
            setBackgroundColor(Color.argb(200, 20, 20, 20))
            setPadding(32, 20, 32, 20)
            textSize = 15f
            visibility = View.GONE
        }
        setContentView(
            FrameLayout(this).apply {
                setBackgroundColor(Color.BLACK)
                addView(view, FrameLayout.LayoutParams(MATCH_PARENT, MATCH_PARENT))
                addView(message, FrameLayout.LayoutParams(WRAP_CONTENT, WRAP_CONTENT, Gravity.TOP or Gravity.CENTER_HORIZONTAL).apply { topMargin = 48 })
            },
        )
        WindowCompat.getInsetsController(window, window.decorView).apply {
            systemBarsBehavior = WindowInsetsControllerCompat.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
            hide(WindowInsetsCompat.Type.systemBars())
        }

        // The device's own decoders first; FFmpeg (module :ffmpeg, LGPL build) only for formats the
        // device cannot decode, e.g. TrueHD and DTS on a Pixel 7 (measured 2026-11-12).
        val store = (application as MoviesCastApp).store
        val renderers = MoviesCastRenderersFactory(this) { store.passthroughOff }
            .setEnableDecoderFallback(true)
            .setExtensionRendererMode(DefaultRenderersFactory.EXTENSION_RENDERER_MODE_ON)
        AppLog.i("ffmpeg ${if (FfmpegLibrary.isAvailable()) "available ${FfmpegLibrary.getVersion()}" else "not available"}")
        val exo = ExoPlayer.Builder(this, renderers)
            .setMediaSourceFactory(DefaultMediaSourceFactory(OkHttpDataSource.Factory(app.mediaHttp)))
            .setAudioAttributes(
                AudioAttributes.Builder().setUsage(C.USAGE_MEDIA).setContentType(C.AUDIO_CONTENT_TYPE_MOVIE).build(),
                true,
            )
            .setHandleAudioBecomingNoisy(true)
            .setSeekBackIncrementMs(10_000)
            .setSeekForwardIncrementMs(30_000)
            .build()
        exo.trackSelectionParameters = exo.trackSelectionParameters.buildUpon()
            .setPreferredAudioLanguage(Locale.getDefault().isO3Language)
            .build()
        exo.addListener(Events())
        exo.addAnalyticsListener(Diagnostics())
        val media = queue.map { entry ->
            ExoMediaItem.Builder()
                .setUri(app.client.streamUrl(session, entry.id))
                .setMediaId(entry.id)
                .setMediaMetadata(MediaMetadata.Builder().setTitle(entry.title).build())
                // Subtitle files lying next to the film; they come from the same server with the same login header.
                .setSubtitleConfigurations(
                    entry.subtitles.map {
                        ExoMediaItem.SubtitleConfiguration.Builder(Uri.parse(it.url))
                            .setMimeType(subtitleMime(it.format))
                            .setLanguage(it.language)
                            .setLabel(it.label)
                            .build()
                    },
                )
                .build()
        }
        val index = (savedInstanceState?.getInt(STATE_INDEX) ?: 0).coerceIn(0, queue.size - 1)
        startMs = savedInstanceState?.getLong(STATE_POSITION) ?: (intent.getLongExtra(EXTRA_START, 0) * 1000)
        itemId = queue[index].id
        AppLog.i("queue of ${queue.size}, subtitle files ${queue[index].subtitles.size}")
        exo.setMediaItems(media, index, startMs)
        exo.playWhenReady = true
        exo.prepare()
        view.player = exo
        player = exo
        mediaSession = MediaSession.Builder(this, exo).build()

        lifecycleScope.launch {
            while (isActive) {
                delay(PROGRESS_EVERY_MS)
                if (reported) report { s, p -> app.client.reportProgress(s, p) }
            }
        }
    }

    override fun onSaveInstanceState(outState: Bundle) {
        super.onSaveInstanceState(outState)
        player?.let {
            outState.putLong(STATE_POSITION, it.currentPosition)
            outState.putInt(STATE_INDEX, it.currentMediaItemIndex)
        }
    }

    /** Home or the task switcher during playback: keep the film in a small window (Android 8 to 11; 12 and later enter by themselves). */
    override fun onUserLeaveHint() {
        super.onUserLeaveHint()
        if (Build.VERSION.SDK_INT in Build.VERSION_CODES.O until Build.VERSION_CODES.S && player?.isPlaying == true) {
            pictureInPicture()?.let { enterPictureInPictureMode(it) }
        }
    }

    override fun onPictureInPictureModeChanged(isInPictureInPictureMode: Boolean, newConfig: Configuration) {
        super.onPictureInPictureModeChanged(isInPictureInPictureMode, newConfig)
        playerView.useController = !isInPictureInPictureMode
        if (isInPictureInPictureMode) message.visibility = View.GONE
        // Closed with the small window's X: the activity is stopped already and nothing would bring it back.
        else if (lifecycle.currentState == Lifecycle.State.CREATED) finish()
    }

    /** The small window's shape and whether Android may open it by itself; null where the device has none. */
    private fun pictureInPicture(): PictureInPictureParams? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O || !packageManager.hasSystemFeature(PackageManager.FEATURE_PICTURE_IN_PICTURE)) return null
        val builder = PictureInPictureParams.Builder()
        val size = player?.videoSize ?: VideoSize.UNKNOWN
        if (size.width > 0 && size.height > 0) builder.setAspectRatio(pipShape(size.width, size.height).let { Rational(it.first, it.second) })
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) builder.setAutoEnterEnabled(player?.isPlaying == true)
        return builder.build()
    }

    private fun updatePictureInPicture() {
        pictureInPicture()?.let { setPictureInPictureParams(it) }
    }

    override fun onStop() {
        super.onStop()
        val exo = player ?: return
        exo.pause()
        if (reported) {
            reported = false
            // On the app's scope: this activity may be gone before the answer arrives.
            report { s, p -> app.client.reportStopped(s, p) }
        }
    }

    override fun onDestroy() {
        mediaSession?.release()
        player?.release()
        player = null
        super.onDestroy()
    }

    private fun state(): PlayState? {
        val exo = player ?: return null
        return PlayState(itemId, playSessionId, exo.currentPosition.coerceAtLeast(0) / 1000, !exo.isPlaying)
    }

    private fun report(send: suspend (Session, PlayState) -> Unit) = report(state(), send)

    private fun report(state: PlayState?, send: suspend (Session, PlayState) -> Unit) {
        if (state == null) return
        app.appScope.launch {
            try {
                send(session, state)
            } catch (e: IOException) {
                AppLog.w("report failed: ${e.javaClass.simpleName}")
            }
        }
    }

    /**
     * The device cannot decode this film's picture: ask the server for a converted stream
     * (H.264 and AAC over HLS) and continue at the same place. Tried once per film; returns
     * false when it was tried before, so the caller shows the error instead.
     */
    private fun askServerToConvert(): Boolean {
        if (converting) return false
        converting = true
        val exo = player ?: return false
        // The film was opened to continue at a place it may not have reached before the decoder gave up.
        val position = exo.currentPosition.coerceAtLeast(if (exo.currentMediaItemIndex == 0) startMs else 0)
        show(getString(R.string.player_converting))
        lifecycleScope.launch {
            val url = try {
                val (width, height) = largestH264Picture()
                AppLog.i("asking for h264 up to ${width}x$height")
                app.client.transcodeUrl(session, itemId, width, height)
            } catch (e: IOException) {
                AppLog.w("converted stream: ${e.javaClass.simpleName}")
                null
            }
            if (url == null) {
                show(getString(R.string.player_cannot_convert))
                return@launch
            }
            AppLog.i("playing the server's converted stream")
            // Only this item is replaced; the episodes after it stay in the queue.
            val at = exo.currentMediaItemIndex
            exo.replaceMediaItem(at, ExoMediaItem.Builder().setUri(url).setMimeType(MimeTypes.APPLICATION_M3U8).setMediaId(itemId).build())
            exo.seekTo(at, position)
            exo.prepare()
            exo.playWhenReady = true
        }
        return true
    }

    /** The largest H.264 picture any decoder of this device takes; 0 x 0 when none says. */
    private fun largestH264Picture(): Pair<Int, Int> {
        var width = 0
        var height = 0
        for (info in MediaCodecList(MediaCodecList.REGULAR_CODECS).codecInfos) {
            if (info.isEncoder) continue
            val video = try {
                info.getCapabilitiesForType(MimeTypes.VIDEO_H264).videoCapabilities
            } catch (_: IllegalArgumentException) {
                null
            } ?: continue
            width = maxOf(width, video.supportedWidths.upper)
            height = maxOf(height, video.supportedHeights.upper)
        }
        return width to height
    }

    private fun show(text: String) {
        message.text = text
        message.visibility = View.VISIBLE
    }

    /**
     * Once for each film, before the viewer chooses: the sound track Kodi would play ([AudioPick]), among those the
     * device can play; a choice in the player's menu afterwards stays the viewer's.
     */
    private fun pickSound(tracks: Tracks) {
        val exo = player ?: return
        val item = exo.currentMediaItem?.mediaId ?: return
        if (item == soundPickedFor) return
        val candidates = tracks.groups.filter { it.type == C.TRACK_TYPE_AUDIO }.flatMap { group ->
            (0 until group.length).filter { group.isTrackSupported(it) }.map { group to it }
        }
        if (candidates.isEmpty()) return
        soundPickedFor = item
        val formats = candidates.map { (group, i) -> group.getTrackFormat(i) }
        val best = AudioPick.best(
            formats.map { AudioPick.Track(it.language, it.sampleMimeType, it.channelCount, it.selectionFlags and C.SELECTION_FLAG_DEFAULT != 0) },
            Locale.getDefault().language,
        )
        AppLog.i("sound tracks: " + formats.joinToString { "${it.language} ${it.sampleMimeType?.substringAfter('/')} ${it.channelCount}ch${if (it.selectionFlags and C.SELECTION_FLAG_DEFAULT != 0) " default" else ""}" } + "; chosen ${best + 1}")
        if (best < 0) return
        val (group, index) = candidates[best]
        if (group.isTrackSelected(index)) return
        exo.trackSelectionParameters = exo.trackSelectionParameters.buildUpon()
            .setOverrideForType(TrackSelectionOverride(group.mediaTrackGroup, index))
            .build()
    }

    private var soundPickedFor: String? = null

    private inner class Events : Player.Listener {
        override fun onIsPlayingChanged(isPlaying: Boolean) {
            if (isPlaying && !reported) {
                reported = true
                report { s, p -> app.client.reportStart(s, p) }
            } else if (reported) {
                report { s, p -> app.client.reportProgress(s, p) }
            }
            updatePictureInPicture()
        }

        override fun onVideoSizeChanged(videoSize: VideoSize) = updatePictureInPicture()

        /**
         * The next item of the queue began (by itself at the end of an episode, or with the
         * next/previous button): the server hears where the old one stopped, at its end when it ran
         * out, which marks it as watched, and the new one gets a play session of its own.
         */
        override fun onPositionDiscontinuity(oldPosition: Player.PositionInfo, newPosition: Player.PositionInfo, reason: Int) {
            if (oldPosition.mediaItemIndex == newPosition.mediaItemIndex) return
            val exo = player ?: return
            if (reported) {
                report(PlayState(itemId, playSessionId, oldPosition.positionMs.coerceAtLeast(0) / 1000, false)) { s, p -> app.client.reportStopped(s, p) }
            }
            itemId = exo.currentMediaItem?.mediaId ?: return
            playSessionId = UUID.randomUUID().toString()
            converting = false
            message.visibility = View.GONE
            AppLog.i("next item of the queue (${newPosition.mediaItemIndex + 1} of ${exo.mediaItemCount})")
            reported = exo.isPlaying
            if (reported) report { s, p -> app.client.reportStart(s, p) }
        }

        override fun onPlaybackStateChanged(playbackState: Int) {
            if (playbackState == Player.STATE_ENDED) finish()
        }

        override fun onRenderedFirstFrame() {
            if (converting) message.visibility = View.GONE
            AppLog.i("first frame ${SystemClock.elapsedRealtime() - openedAt} ms after opening")
        }

        override fun onPlayerError(error: PlaybackException) {
            AppLog.e("playback error ${error.errorCodeName}")
            if (error.errorCode in DECODER_ERRORS && askServerToConvert()) return
            show(getString(R.string.player_error, error.errorCodeName))
        }

        // A file can hold a sound or picture format this device has no decoder for; the player
        // would then run silent or black without a word.
        override fun onTracksChanged(tracks: Tracks) {
            for ((type, text) in listOf(C.TRACK_TYPE_AUDIO to R.string.player_no_audio_decoder, C.TRACK_TYPE_VIDEO to R.string.player_no_video_decoder)) {
                val groups = tracks.groups.filter { it.type == type }
                if (groups.isNotEmpty() && groups.none { it.isSupported }) {
                    val format = groups.first().getTrackFormat(0)
                    AppLog.w("no decoder for ${format.sampleMimeType} ${format.codecs.orEmpty()}")
                    // No picture is no film: let the server convert. Missing sound only gets the message.
                    if (type == C.TRACK_TYPE_VIDEO && askServerToConvert()) return
                    show(getString(text, format.sampleMimeType?.substringAfter('/')?.uppercase() ?: "?"))
                    return
                }
            }
            if (!converting) message.visibility = View.GONE
            pickSound(tracks)
            val text = tracks.groups.filter { it.type == C.TRACK_TYPE_TEXT }
            if (text.isNotEmpty()) AppLog.i("subtitle tracks ${text.size}: " + text.joinToString { it.getTrackFormat(0).language ?: "?" })
        }
    }

    /** One log line per decoder and format, so a tester's `adb logcat -s MoviesCast` says what the device did. */
    private inner class Diagnostics : AnalyticsListener {
        override fun onAudioDecoderInitialized(eventTime: AnalyticsListener.EventTime, decoderName: String, initializedTimestampMs: Long, initializationDurationMs: Long) {
            AppLog.i("audio decoder $decoderName")
        }

        override fun onVideoDecoderInitialized(eventTime: AnalyticsListener.EventTime, decoderName: String, initializedTimestampMs: Long, initializationDurationMs: Long) {
            AppLog.i("video decoder $decoderName")
        }

        override fun onAudioInputFormatChanged(eventTime: AnalyticsListener.EventTime, format: Format, decoderReuseEvaluation: androidx.media3.exoplayer.DecoderReuseEvaluation?) {
            AppLog.i("audio format ${format.sampleMimeType} channels=${format.channelCount} lang=${format.language}")
        }

        override fun onVideoInputFormatChanged(eventTime: AnalyticsListener.EventTime, format: Format, decoderReuseEvaluation: androidx.media3.exoplayer.DecoderReuseEvaluation?) {
            AppLog.i("video format ${format.sampleMimeType} ${format.width}x${format.height}")
        }
    }

    companion object {
        private const val EXTRA_QUEUE = "queue"
        private const val STATE_INDEX = "index"
        private const val EXTRA_START = "start"
        private const val STATE_POSITION = "position"
        private const val PROGRESS_EVERY_MS = 10_000L
        private val DECODER_ERRORS = setOf(
            PlaybackException.ERROR_CODE_DECODER_INIT_FAILED,
            PlaybackException.ERROR_CODE_DECODER_QUERY_FAILED,
            PlaybackException.ERROR_CODE_DECODING_FAILED,
            PlaybackException.ERROR_CODE_DECODING_FORMAT_EXCEEDS_CAPABILITIES,
            PlaybackException.ERROR_CODE_DECODING_FORMAT_UNSUPPORTED,
        )

        /**
         * Width and height for the small window: the picture's own shape, held inside what Android
         * accepts (between 1:2.39 and 2.39:1, otherwise it throws).
         */
        internal fun pipShape(width: Int, height: Int): Pair<Int, Int> = when {
            width * 100 > height * 239 -> 239 to 100
            height * 100 > width * 239 -> 100 to 239
            else -> width to height
        }

        /** The MIME type Media3 reads a subtitle file of this format with. */
        internal fun subtitleMime(format: String): String = when (format) {
            "vtt" -> MimeTypes.TEXT_VTT
            "ass", "ssa" -> MimeTypes.TEXT_SSA
            else -> MimeTypes.APPLICATION_SUBRIP
        }

        /** Plays the first entry of [queue] from [fromSeconds] and the others after it. */
        fun intent(context: Context, queue: List<QueueEntry>, fromSeconds: Long): Intent =
            Intent(context, PlayerActivity::class.java)
                .putExtra(EXTRA_QUEUE, ArrayList(queue))
                .putExtra(EXTRA_START, fromSeconds)
    }
}
