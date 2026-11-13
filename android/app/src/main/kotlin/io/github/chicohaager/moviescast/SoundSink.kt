package io.github.chicohaager.moviescast

import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.audio.AudioSink
import androidx.media3.exoplayer.audio.AudioSink.AudioSinkConfig
import androidx.media3.exoplayer.audio.ForwardingAudioSink
import io.github.chicohaager.moviescast.core.Passthrough

/**
 * Media3's audio sink, except that a sound format the person turned off ([Passthrough]) is never handed on
 * unchanged: the sink says it cannot take it, so Media3 decodes it to PCM (a decoder of the device, else FFmpeg).
 * Media3 asks the sink before it passes sound on (MediaCodecAudioRenderer: `audioSink.supportsFormat`).
 * Each configuration is logged, so the diagnostics say whether the sound went out as PCM or unchanged.
 */
@UnstableApi
class SoundSink(sink: AudioSink, private val off: () -> Set<Passthrough.Format>) : ForwardingAudioSink(sink) {

    override fun supportsFormat(format: Format): Boolean =
        Passthrough.mayPassOn(format.sampleMimeType, off()) && super.supportsFormat(format)

    override fun getFormatSupport(format: Format): Int =
        if (Passthrough.mayPassOn(format.sampleMimeType, off())) super.getFormatSupport(format) else AudioSink.SINK_FORMAT_UNSUPPORTED

    override fun configure(audioSinkConfig: AudioSinkConfig) {
        val format = audioSinkConfig.format
        AppLog.i(
            if (format.sampleMimeType == MimeTypes.AUDIO_RAW) "audio output pcm channels=${format.channelCount} rate=${format.sampleRate}"
            else "audio output passthrough ${format.sampleMimeType}",
        )
        super.configure(audioSinkConfig)
    }
}
