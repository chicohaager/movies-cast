package io.github.chicohaager.moviescast

import androidx.media3.common.Format
import androidx.media3.common.MimeTypes
import androidx.media3.common.util.UnstableApi
import androidx.media3.exoplayer.audio.AudioSink
import io.github.chicohaager.moviescast.core.Passthrough
import io.github.chicohaager.moviescast.core.Passthrough.Format as Sound
import java.lang.reflect.Proxy
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

@UnstableApi
class SoundSinkTest {
    /** Stands in for Media3's sink on a device that says it passes every format on (as the Fire TV stick did for DTS-HD). */
    private val takesEverything = Proxy.newProxyInstance(AudioSink::class.java.classLoader, arrayOf(AudioSink::class.java)) { _, method, _ ->
        when (method.name) {
            "supportsFormat" -> true
            "getFormatSupport" -> AudioSink.SINK_FORMAT_SUPPORTED_DIRECTLY
            else -> null
        }
    } as AudioSink

    private fun format(mime: String) = Format.Builder().setSampleMimeType(mime).setChannelCount(6).setSampleRate(48000).build()

    @Test
    fun media3sNamesOfEverySoundFormatBelongToTheirSetting() {
        val expected = mapOf(
            MimeTypes.AUDIO_AC3 to Sound.AC3,
            MimeTypes.AUDIO_E_AC3 to Sound.EAC3,
            MimeTypes.AUDIO_E_AC3_JOC to Sound.EAC3,
            MimeTypes.AUDIO_DTS to Sound.DTS,
            MimeTypes.AUDIO_DTS_HD to Sound.DTS,
            MimeTypes.AUDIO_DTS_EXPRESS to Sound.DTS,
            MimeTypes.AUDIO_DTS_UHD_P2 to Sound.DTS,
            MimeTypes.AUDIO_TRUEHD to Sound.TRUEHD,
        )
        for ((mime, sound) in expected) assertEquals(mime, sound, Passthrough.formatOf(mime))
    }

    @Test
    fun aFormatTurnedOffIsRefusedSoMedia3DecodesIt() {
        val sink = SoundSink(takesEverything) { setOf(Sound.DTS) }
        for (mime in listOf(MimeTypes.AUDIO_DTS, MimeTypes.AUDIO_DTS_HD)) {
            assertFalse(mime, sink.supportsFormat(format(mime)))
            assertEquals(mime, AudioSink.SINK_FORMAT_UNSUPPORTED, sink.getFormatSupport(format(mime)))
        }
    }

    @Test
    fun formatsNotTurnedOffAndPcmAreLeftToTheDevice() {
        val sink = SoundSink(takesEverything) { setOf(Sound.DTS) }
        for (mime in listOf(MimeTypes.AUDIO_TRUEHD, MimeTypes.AUDIO_AC3, MimeTypes.AUDIO_E_AC3, MimeTypes.AUDIO_RAW)) {
            assertTrue(mime, sink.supportsFormat(format(mime)))
            assertEquals(mime, AudioSink.SINK_FORMAT_SUPPORTED_DIRECTLY, sink.getFormatSupport(format(mime)))
        }
    }

    @Test
    fun theSettingIsReadAtEachQuestionNotOnce() {
        var off = emptySet<Sound>()
        val sink = SoundSink(takesEverything) { off }
        assertTrue(sink.supportsFormat(format(MimeTypes.AUDIO_TRUEHD)))
        off = setOf(Sound.TRUEHD)
        assertFalse(sink.supportsFormat(format(MimeTypes.AUDIO_TRUEHD)))
    }
}
