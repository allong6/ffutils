#!/bin/bash
# FFBox 裁剪版 ffmpeg/ffprobe 构建脚本(正式资产)
#
# 策略:只砍输出(encoder/muxer 白名单),输入面(decoder/demuxer/filter)全保留,
# 保证"任意素材能打开"的产品承诺。白名单来源:ffutils enums.go 全量枚举 +
# GUI service 层输出格式普查(mp4/mkv/webm/mp3/wav/flac/ogg/aac/m4a/gif/ts +
# png/jpg 截图)+ 硬件编码三家探测 + 字幕烧录(libass)+ HLS 双向(hls/segment,
# FromHLS 需 http 协议故不禁 network)。
#
# 运行环境:MSYS2 mingw64 shell(自检 MSYSTEM)。首次运行自动装齐 pacman 依赖、
# 下载 NVENC 头文件与 ffmpeg 源码;脚本幂等,重复运行增量编译。
# 用法:tools/build-ffmpeg/build-trim.sh [工作目录](默认脚本旁 work/,已 gitignore)
# 产物:$WORK/out/bin/{ffmpeg,ffprobe}.exe + manifest.json(build facts,
#       校验/分发/合规凭据;验证结论记录在 README 与 ffbox 仓 docs/STATUS.md)
#
# 已知坑(改动 configure 前必读,详见 README"踩坑记录"):
#   lavfi 输入属 avdevice、nvenc 需显式 enable、PCM 封装改名 pcm_s16le、
#   vpl.pc/x265.pc 两处 MSYS2 打包瑕疵需修补(本脚本自动做)。
set -euo pipefail

FFVER="8.1.2"
NVCODEC_VER="13.0.19.1"

[ "${MSYSTEM:-}" = "MINGW64" ] || { echo "错误:请在 MSYS2 mingw64 shell 运行(当前 MSYSTEM=${MSYSTEM:-未设置})"; exit 1; }

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
WORK="${1:-$SCRIPT_DIR/work}"
mkdir -p "$WORK"
cd "$WORK"
echo "== 工作目录: $WORK"

# --- 1. pacman 依赖(幂等;镜像建议先切国内源,见 README) ---
pacman -S --needed --noconfirm \
  mingw-w64-x86_64-gcc mingw-w64-x86_64-binutils mingw-w64-x86_64-pkgconf \
  make diffutils patch nasm git \
  mingw-w64-x86_64-x264 mingw-w64-x86_64-x265 mingw-w64-x86_64-libvpx \
  mingw-w64-x86_64-libtheora mingw-w64-x86_64-libvorbis mingw-w64-x86_64-libogg \
  mingw-w64-x86_64-opus mingw-w64-x86_64-lame mingw-w64-x86_64-dav1d \
  mingw-w64-x86_64-libvpl mingw-w64-x86_64-amf-headers mingw-w64-x86_64-zlib \
  mingw-w64-x86_64-libass mingw-w64-x86_64-gnutls > /dev/null

# --- 2. NVENC 头文件(MSYS2 未打包,来自 FFmpeg/nv-codec-headers) ---
if [ ! -f /mingw64/include/ffnvcodec/nvEncodeAPI.h ]; then
  echo "== 下载 nv-codec-headers $NVCODEC_VER"
  curl -sL -o nv-headers.tar.gz \
    "https://github.com/FFmpeg/nv-codec-headers/releases/download/n${NVCODEC_VER}/nv-codec-headers-${NVCODEC_VER}.tar.gz"
  tar -xf nv-headers.tar.gz
  cp -r "nv-codec-headers-${NVCODEC_VER}/include/ffnvcodec" /mingw64/include/
  printf 'Name: ffnvcodec\nDescription: FFmpeg CUDA headers\nVersion: %s\nCflags: -I${pcfiledir}/../../include/ffnvcodec\n' \
    "$NVCODEC_VER" > /mingw64/lib/pkgconfig/ffnvcodec.pc
fi

# --- 3. MSYS2 打包瑕疵修补(必须随脚本固化,保证任何机器可复现) ---
# a) libvpl.a 含 C++ 对象,vpl.pc 的 Libs.private 缺 -lstdc++,链接失败
grep -q -- '-lstdc++' /mingw64/lib/pkgconfig/vpl.pc || \
  sed -i 's/^Libs.private:.*/& -lstdc++/' /mingw64/lib/pkgconfig/vpl.pc
# b) x265.pc 的 Libs.private 带 -lgcc_s/-lgcc:显式注入动态 libgcc,与 -static
#    冲突产生 _Unwind_Resume 重复定义
sed -i 's/^Libs.private:.*/Libs.private: -lstdc++/' /mingw64/lib/pkgconfig/x265.pc

# --- 4. 源码(下载/复用) ---
if [ ! -d "ffmpeg-${FFVER}" ]; then
  [ -f "ffmpeg-${FFVER}.tar.xz" ] || curl -sL -O "https://ffmpeg.org/releases/ffmpeg-${FFVER}.tar.xz"
  tar -xf "ffmpeg-${FFVER}.tar.xz"
fi
cd "ffmpeg-${FFVER}"

# --- 5. configure + make ---
export PKG_CONFIG_PATH=/mingw64/lib/pkgconfig

CONFIGURE_ARGS=(
  --prefix="$WORK/out"
  --pkg-config-flags=--static
  --enable-static --disable-shared
  --extra-ldflags="-static"
  --enable-gpl --enable-version3
  --disable-autodetect
  --disable-ffplay --disable-doc --disable-debug
  --disable-devices --enable-indev=lavfi
  --enable-zlib
  --enable-libx264 --enable-libx265 --enable-libvpx --enable-libtheora
  --enable-libvorbis --enable-libopus --enable-libmp3lame --enable-libdav1d
  --enable-libvpl --enable-amf --enable-ffnvcodec --enable-nvenc
  --enable-libass --enable-gnutls
  --disable-encoders
  --enable-encoder=libx264,libx265,libvpx_vp8,libvpx_vp9,libtheora,gif,prores_ks,png,mjpeg,bmp,wrapped_avframe,h264_nvenc,hevc_nvenc,h264_qsv,h264_amf,aac,libmp3lame,libopus,libvorbis,ac3,flac,pcm_s16le
  --disable-muxers
  --enable-muxer=mp4,mov,ipod,matroska,webm,ogg,mp3,adts,wav,flac,ac3,gif,avi,mpegts,image2,image2pipe,null,pcm_s16le,hls,segment
)

echo "== configure(白名单见 manifest.json)"
./configure "${CONFIGURE_ARGS[@]}" > configure-last.log 2>&1 || { tail -20 configure-last.log; exit 1; }

echo "== make -j$(nproc)"
make -j"$(nproc)" > build-last.log 2>&1 || { tail -30 build-last.log; exit 1; }
make install >> build-last.log 2>&1

# --- 6. manifest.json(构建事实:版本/参数/hash/体积/依赖版本) ---
FFHASH=$(sha256sum "$WORK/out/bin/ffmpeg.exe" | cut -d' ' -f1)
PFHASH=$(sha256sum "$WORK/out/bin/ffprobe.exe" | cut -d' ' -f1)
FFSIZE=$(stat -c%s "$WORK/out/bin/ffmpeg.exe")
PFSIZE=$(stat -c%s "$WORK/out/bin/ffprobe.exe")
GCCVER=$(gcc -dumpversion)
BUILDDATE=$(date +%F)
LIBVER=$(pacman -Q mingw-w64-x86_64-x264 mingw-w64-x86_64-x265 mingw-w64-x86_64-libvpx \
  mingw-w64-x86_64-libtheora mingw-w64-x86_64-libvorbis mingw-w64-x86_64-opus \
  mingw-w64-x86_64-lame mingw-w64-x86_64-dav1d mingw-w64-x86_64-libvpl \
  mingw-w64-x86_64-libass mingw-w64-x86_64-gnutls \
  | awk '{printf "    \"%s\": \"%s\"\n", $1, $2}' | paste -sd ',' | sed 's/,/,\n/g')

cat > "$WORK/out/manifest.json" <<EOF
{
  "ffmpeg_version": "$FFVER",
  "nv_codec_headers": "$NVCODEC_VER",
  "build_date": "$BUILDDATE",
  "compiler": "gcc $GCCVER (MSYS2 mingw-w64)",
  "libraries": {
$LIBVER
  },
  "configure": "./configure ${CONFIGURE_ARGS[*]}",
  "artifacts": {
    "ffmpeg.exe":  { "sha256": "$FFHASH", "size": $FFSIZE },
    "ffprobe.exe": { "sha256": "$PFHASH", "size": $PFSIZE }
  }
}
EOF

echo "== 构建完成"
ls -la "$WORK/out/bin/" | grep exe
echo "== manifest: $WORK/out/manifest.json"
