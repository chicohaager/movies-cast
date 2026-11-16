package io.github.chicohaager.moviescast.core

import io.github.chicohaager.moviescast.core.AudioPick.Track
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class AudioPickTest {
    @Test
    fun theFilesDefaultInTheDevicesLanguageBeatsAMonoTrack() {
        // "Der kleine Horrorladen" on Movies (2026-11-16): DTS-HD 5.1 de (default), DTS-HD 5.1 en, AC3 mono de
        val tracks = listOf(
            Track("de", "audio/vnd.dts.hd", 6, isDefault = true),
            Track("en", "audio/vnd.dts.hd", 6, isDefault = false),
            Track("de", "audio/ac3", 1, isDefault = false),
        )
        assertEquals(0, AudioPick.best(tracks, "de"))
        // the same with the mono track first in the file and no track marked
        val unmarked = listOf(tracks[2], tracks[1], tracks[0].copy(isDefault = false))
        assertEquals(2, AudioPick.best(unmarked, "de"))
    }

    @Test
    fun theDevicesLanguageComesBeforeTheDefault() {
        val tracks = listOf(Track("eng", "audio/true-hd", 8, isDefault = true), Track("ger", "audio/ac3", 6, isDefault = false))
        assertEquals(1, AudioPick.best(tracks, "de"))
        assertEquals(0, AudioPick.best(tracks, "en"))
        // no track in the device's language: the default
        assertEquals(0, AudioPick.best(tracks, "fr"))
    }

    @Test
    fun theDefaultComesBeforeQuality() {
        // Kodi: the default flag is a tier above quality, so a default AC3 5.1 beats an unmarked TrueHD 7.1
        val tracks = listOf(Track("de", "audio/true-hd", 8, isDefault = false), Track("de", "audio/ac3", 6, isDefault = true))
        assertEquals(1, AudioPick.best(tracks, "de"))
    }

    @Test
    fun qualityAsKodiRanksIt() {
        val truehd51 = Track(null, "audio/true-hd", 6, false)
        val ac3_71 = Track(null, "audio/ac3", 8, false)
        val aacStereo = Track(null, "audio/mp4a-latm", 2, false)
        val ac3Mono = Track(null, "audio/ac3", 1, false)
        // beyond stereo the codec counts before the channels: 5.1 TrueHD over 7.1 AC3
        assertTrue(AudioPick.compareQuality(truehd51, ac3_71) > 0)
        // any surround track over stereo, stereo over mono whatever the codec
        assertTrue(AudioPick.compareQuality(ac3_71, aacStereo) > 0)
        assertTrue(AudioPick.compareQuality(aacStereo, ac3Mono) > 0)
        assertEquals(0, AudioPick.best(listOf(truehd51, ac3_71, aacStereo, ac3Mono), "de"))
    }

    @Test
    fun equalTracksKeepTheFilesOrderAndNoneIsNone() {
        val t = Track("de", "audio/eac3", 6, false)
        assertEquals(0, AudioPick.best(listOf(t, t.copy()), "de"))
        assertEquals(-1, AudioPick.best(emptyList(), "de"))
    }
}
