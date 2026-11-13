package io.github.chicohaager.moviescast.core

import io.github.chicohaager.moviescast.core.Passthrough.Format
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** The MIME types are Media3's (MimeTypes.AUDIO_*), written out: core knows no Media3. */
class PassthroughTest {
    @Test
    fun everyDtsKindBelongsToTheDtsSetting() {
        for (mime in listOf("audio/vnd.dts", "audio/vnd.dts.hd", "audio/vnd.dts.hd;profile=lbr", "audio/vnd.dts.uhd;profile=p2")) {
            assertEquals(mime, Format.DTS, Passthrough.formatOf(mime))
        }
    }

    @Test
    fun dolbyFormatsBelongToTheirOwnSettings() {
        assertEquals(Format.AC3, Passthrough.formatOf("audio/ac3"))
        assertEquals(Format.EAC3, Passthrough.formatOf("audio/eac3"))
        assertEquals(Format.EAC3, Passthrough.formatOf("audio/eac3-joc"))
        assertEquals(Format.TRUEHD, Passthrough.formatOf("audio/true-hd"))
    }

    @Test
    fun soundThatIsNoSuchFormatIsNeverHeldBack() {
        for (mime in listOf("audio/raw", "audio/mp4a-latm", "audio/opus", "audio/flac", null)) {
            assertNull(Passthrough.formatOf(mime))
            assertTrue(Passthrough.mayPassOn(mime, Format.entries.toSet()))
        }
    }

    @Test
    fun aFormatTurnedOffMayNotBeHandedOnTheOthersMay() {
        val off = setOf(Format.DTS)
        assertFalse(Passthrough.mayPassOn("audio/vnd.dts.hd", off))
        assertFalse(Passthrough.mayPassOn("audio/vnd.dts", off))
        assertTrue(Passthrough.mayPassOn("audio/true-hd", off))
        assertTrue(Passthrough.mayPassOn("audio/ac3", off))
    }

    @Test
    fun withNothingTurnedOffEverythingIsLeftToTheDevice() {
        for (format in Format.entries) for (mime in format.mimeTypes) assertTrue(mime, Passthrough.mayPassOn(mime, emptySet()))
    }

    @Test
    fun theSettingIsKeptAsNamesAndReadBack() {
        val off = setOf(Format.TRUEHD, Format.DTS)
        assertEquals("dts,truehd", Passthrough.encode(off))
        assertEquals(off, Passthrough.decode("dts,truehd"))
        // "every format passed on" is kept as "" and read back as such, not as the default
        assertEquals(emptySet<Format>(), Passthrough.decode(""))
        assertEquals(setOf(Format.AC3), Passthrough.decode("ac3, unknown"))
    }

    /** As Kodi 21 ships (audiooutput.passthrough and each format's switch default false): nothing set means every
     *  format is decoded in the app, so the sound plays on a TV that cannot take what the box would pass on. */
    @Test
    fun withNothingSetEveryFormatIsDecodedInTheApp() {
        assertEquals(Format.entries.toSet(), Passthrough.decode(null))
        for (format in Format.entries) for (mime in format.mimeTypes) assertFalse(mime, Passthrough.mayPassOn(mime, Passthrough.decode(null)))
    }
}
