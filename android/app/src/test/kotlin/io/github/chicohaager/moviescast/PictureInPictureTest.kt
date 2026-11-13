package io.github.chicohaager.moviescast

import org.junit.Assert.assertEquals
import org.junit.Test

class PictureInPictureTest {
    @Test
    fun theSmallWindowKeepsThePicturesShapeInsideWhatAndroidAccepts() {
        assertEquals(1920 to 1080, PlayerActivity.pipShape(1920, 1080))
        assertEquals(3840 to 1608, PlayerActivity.pipShape(3840, 1608)) // 2.388:1
    }

    @Test
    fun aPictureWiderOrTallerThanAndroidAcceptsIsHeldAtTheLimit() {
        assertEquals(239 to 100, PlayerActivity.pipShape(3840, 1600))
        assertEquals(239 to 100, PlayerActivity.pipShape(3840, 1606)) // 2.391:1, the film measured from Movies
        assertEquals(100 to 239, PlayerActivity.pipShape(1000, 2400))
        assertEquals(1000 to 2390, PlayerActivity.pipShape(1000, 2390))
    }
}
