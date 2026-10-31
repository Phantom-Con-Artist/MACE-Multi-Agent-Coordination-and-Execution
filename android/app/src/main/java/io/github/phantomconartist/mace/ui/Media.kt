package io.github.phantomconartist.mace.ui

import android.content.Context
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.media.AudioAttributes
import android.media.AudioManager
import android.media.MediaMetadataRetriever
import android.media.SoundPool
import android.util.LruCache
import androidx.compose.foundation.Image
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.aspectRatio
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.layout.widthIn
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Folder
import androidx.compose.material.icons.rounded.Image
import androidx.compose.material.icons.rounded.InsertDriveFile
import androidx.compose.material.icons.rounded.PictureAsPdf
import androidx.compose.material.icons.rounded.PlayArrow
import androidx.compose.material.icons.rounded.Videocam
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.ImageBitmap
import androidx.compose.ui.graphics.asImageBitmap
import androidx.compose.ui.layout.ContentScale
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.Dp
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import io.github.phantomconartist.mace.Member
import io.github.phantomconartist.mace.data.Hub
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext

// Pictures and videos in chats load from the hub with the phone's
// credential, shrink to the size they are shown at, and stay in memory
// while the app runs. Other files open in the phone's own apps.

enum class FileKind { Image, Video, Pdf, Folder, Other }

private val imageExt = setOf("png", "jpg", "jpeg", "webp", "gif", "bmp", "heic", "heif")
private val videoExt = setOf("mp4", "mov", "webm", "mkv", "3gp", "m4v")

fun kindOf(path: String): FileKind {
    if (path.endsWith("/")) return FileKind.Folder
    return when (path.substringAfterLast('.', "").lowercase()) {
        in imageExt -> FileKind.Image
        in videoExt -> FileKind.Video
        "pdf" -> FileKind.Pdf
        else -> FileKind.Other
    }
}

class MediaLoader(private val hub: () -> Hub) {
    private val cache = object : LruCache<String, Bitmap>(48 shl 20) {
        override fun sizeOf(key: String, value: Bitmap) = value.byteCount
    }
    private val missing = mutableSetOf<String>()

    /** The photo of an agent, or null when it has none. */
    fun avatarUrl(m: Member): String? = if (m.photo > 0) hub().photoPath(m.id, false) + "?v=${m.photo}" else null

    fun groupPhotoUrl(id: String, photo: Long): String? = if (photo > 0) hub().photoPath(id, true) + "?v=$photo" else null

    fun fileUrl(path: String) = hub().fileUrl(path)

    fun cached(key: String): ImageBitmap? = cache.get(key)?.asImageBitmap()

    /** A picture from the hub, shrunk to about maxPx on its longer side. */
    suspend fun image(url: String, maxPx: Int): ImageBitmap? {
        val key = "$url@$maxPx"
        cache.get(key)?.let { return it.asImageBitmap() }
        if (key in missing) return null
        val bytes = runCatching { hub().bytes(url) }.getOrNull()
        val bmp = bytes?.let { withContext(Dispatchers.Default) { decode(it, maxPx) } }
        if (bmp == null) { missing += key; return null }
        cache.put(key, bmp)
        return bmp.asImageBitmap()
    }

    /** A video's first frame, read over the network without downloading it all. */
    suspend fun videoFrame(path: String, maxPx: Int): ImageBitmap? {
        val key = "frame:$path@$maxPx"
        cache.get(key)?.let { return it.asImageBitmap() }
        if (key in missing) return null
        val frame = withContext(Dispatchers.IO) {
            runCatching {
                MediaMetadataRetriever().use { r ->
                    r.setDataSource(hub().fileUrl(path), hub().headers())
                    r.getScaledFrameAtTime(0, MediaMetadataRetriever.OPTION_CLOSEST_SYNC, maxPx, maxPx)
                }
            }.getOrNull()
        }
        if (frame == null) { missing += key; return null }
        cache.put(key, frame)
        return frame.asImageBitmap()
    }

    fun forget(prefix: String) {
        cache.snapshot().keys.filter { it.startsWith(prefix) }.forEach { cache.remove(it) }
        missing.removeAll { it.startsWith(prefix) }
    }

    private fun decode(bytes: ByteArray, maxPx: Int): Bitmap? {
        val bounds = BitmapFactory.Options().apply { inJustDecodeBounds = true }
        BitmapFactory.decodeByteArray(bytes, 0, bytes.size, bounds)
        if (bounds.outWidth <= 0) return null
        var sample = 1
        while (maxOf(bounds.outWidth, bounds.outHeight) / (sample * 2) >= maxPx) sample *= 2
        return BitmapFactory.decodeByteArray(bytes, 0, bytes.size, BitmapFactory.Options().apply { inSampleSize = sample })
    }
}

/** Loads a picture for a composable; null while loading or when there is none. */
@Composable
fun rememberImage(media: MediaLoader, url: String?, maxPx: Int): ImageBitmap? {
    var img by remember(url, maxPx) { mutableStateOf(url?.let { media.cached("$it@$maxPx") }) }
    LaunchedEffect(url, maxPx) { if (url != null && img == null) img = media.image(url, maxPx) }
    return img
}

/** An avatar showing the photo when there is one. */
@Composable
fun Avatar(name: String, id: String, group: Boolean, size: Int, photo: String?, media: MediaLoader?) {
    val img = if (media != null) rememberImage(media, photo, size * 3) else null
    if (img != null) Image(img, name, contentScale = ContentScale.Crop, modifier = Modifier.size(size.dp).clip(CircleShape))
    else Avatar(name, id, group, size)
}

/** A shared file inside a bubble: pictures and videos as previews, other files as a card. */
@Composable
fun FileCard(path: String, ink: Color, width: Dp, media: MediaLoader, onOpen: (String) -> Unit) {
    val name = path.trimEnd('/').substringAfterLast('/')
    when (kindOf(path)) {
        FileKind.Image, FileKind.Video -> {
            val video = kindOf(path) == FileKind.Video
            var img by remember(path) { mutableStateOf<ImageBitmap?>(null) }
            var tried by remember(path) { mutableStateOf(false) }
            LaunchedEffect(path) {
                img = if (video) media.videoFrame(path, 720) else media.image(media.fileUrl(path), 1080)
                tried = true
            }
            val ratio = img?.let { (it.width.toFloat() / it.height).coerceIn(0.6f, 1.8f) } ?: 1.3f
            Box(
                Modifier.widthIn(max = width - 20.dp).width(width - 20.dp).aspectRatio(ratio).clip(RoundedCornerShape(8.dp))
                    .background(ink.copy(alpha = 0.08f)).clickable { onOpen(path) },
                contentAlignment = Alignment.Center,
            ) {
                img?.let { Image(it, name, contentScale = ContentScale.Crop, modifier = Modifier.fillMaxSize()) }
                    ?: Icon(if (video) Icons.Rounded.Videocam else Icons.Rounded.Image, null, tint = ink.copy(alpha = if (tried) 0.5f else 0.25f), modifier = Modifier.size(40.dp))
                if (video) {
                    Box(Modifier.size(56.dp).clip(CircleShape).background(Color(0x99000000)), contentAlignment = Alignment.Center) {
                        Icon(Icons.Rounded.PlayArrow, "Play", tint = Color.White, modifier = Modifier.size(36.dp))
                    }
                }
            }
            Spacer(Modifier.height(2.dp))
        }
        else -> {
            val kind = kindOf(path)
            Row(
                Modifier.padding(bottom = 4.dp).clip(RoundedCornerShape(8.dp)).background(ink.copy(alpha = 0.08f)).clickable { onOpen(path) }
                    .padding(horizontal = 10.dp, vertical = 10.dp),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Icon(
                    when (kind) { FileKind.Folder -> Icons.Rounded.Folder; FileKind.Pdf -> Icons.Rounded.PictureAsPdf; else -> Icons.Rounded.InsertDriveFile },
                    null, modifier = Modifier.size(32.dp), tint = MaterialTheme.colorScheme.primary,
                )
                Spacer(Modifier.width(10.dp))
                Column(Modifier.widthIn(max = width - 90.dp)) {
                    Text(name, style = MaterialTheme.typography.bodyLarge, fontWeight = FontWeight.Medium, maxLines = 2, overflow = TextOverflow.Ellipsis)
                    Text(
                        when (kind) { FileKind.Folder -> "Folder"; FileKind.Pdf -> "PDF"; else -> name.substringAfterLast('.', "").uppercase().ifEmpty { "File" } },
                        fontSize = 12.sp, color = ink.copy(alpha = 0.65f),
                    )
                }
            }
        }
    }
}

/** Message sounds: a pop when a message leaves, a chime when one arrives. Silent when the phone is. */
class Sounds(context: Context) {
    private val audio = context.getSystemService(AudioManager::class.java)
    private val pool = SoundPool.Builder().setMaxStreams(2)
        .setAudioAttributes(AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_ASSISTANCE_SONIFICATION).setContentType(AudioAttributes.CONTENT_TYPE_SONIFICATION).build())
        .build()
    private val send = pool.load(context, io.github.phantomconartist.mace.R.raw.send, 1)
    private val receive = pool.load(context, io.github.phantomconartist.mace.R.raw.receive, 1)
    var enabled = true

    private fun play(id: Int, volume: Float) {
        if (enabled && audio.ringerMode == AudioManager.RINGER_MODE_NORMAL) pool.play(id, volume, volume, 1, 0, 1f)
    }

    fun sent() = play(send, 0.5f)

    fun received() = play(receive, 0.6f)
}
