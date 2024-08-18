import cv2
import os

def detect_faces(image_path):
    gray = cv2.imread(image_path, cv2.IMREAD_GRAYSCALE)
    if gray is None:
        return 0
    
    face_cascade = cv2.CascadeClassifier(cv2.data.haarcascades + 'haarcascade_frontalface_default.xml')
    
    faces = face_cascade.detectMultiScale(
        gray,
        scaleFactor=1.1,
        minNeighbors=3,
        minSize=(30, 30),
        flags=cv2.CASCADE_SCALE_IMAGE
    )

    if len(faces) > 0:
        color_image = cv2.imread(image_path)
        for (x, y, w, h) in faces:
            cv2.rectangle(color_image, (x, y), (x + w, y + h), (0, 0, 255), 2)
        
        os.makedirs("uploads", exist_ok=True)
        processed_path = os.path.join("uploads", os.path.basename(image_path))
        cv2.imwrite(processed_path, color_image)

    return len(faces)