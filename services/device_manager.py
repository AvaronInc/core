from typing import Dict, List, Optional, Any
from datetime import datetime
from pydantic import BaseModel
import asyncio
import logging

logger = logging.getLogger(__name__)

class DeviceInfo(BaseModel):
    device_id: str
    hostname: str
    ip_address: str
    platform: str
    capabilities: Dict[str, Any]
    model_version: Optional[str] = "mistral:7b"

class DeviceStatus(BaseModel):
    device_id: str
    status: str  # online, offline, updating
    last_seen: datetime
    current_model: str
    metrics: Optional[Dict] = None

class DeviceManager:
    def __init__(self):
        self.devices: Dict[str, DeviceStatus] = {}
        self.update_callbacks = []
        
    async def register_device(self, device_info: DeviceInfo) -> DeviceStatus:
        """Register a new device or update existing"""
        status = DeviceStatus(
            device_id=device_info.device_id,
            status="online",
            last_seen=datetime.now(),
            current_model=device_info.model_version
        )
        
        self.devices[device_info.device_id] = status
        logger.info(f"Registered device: {device_info.device_id}")
        
        # Notify callbacks
        for callback in self.update_callbacks:
            await callback(device_info.device_id, "registered")
        
        return status
    
    async def update_device_status(self, device_id: str, status: str, metrics: Dict = None):
        """Update device status"""
        if device_id in self.devices:
            self.devices[device_id].status = status
            self.devices[device_id].last_seen = datetime.now()
            if metrics:
                self.devices[device_id].metrics = metrics
            
            # Notify callbacks
            for callback in self.update_callbacks:
                await callback(device_id, "status_updated")
    
    async def get_device_status(self, device_id: str) -> Optional[DeviceStatus]:
        """Get current device status"""
        return self.devices.get(device_id)
    
    async def get_all_devices(self) -> List[DeviceStatus]:
        """Get all registered devices"""
        return list(self.devices.values())
    
    async def trigger_model_update(self, device_id: str, new_model: str) -> bool:
        """Trigger model update on device"""
        if device_id not in self.devices:
            return False
        
        self.devices[device_id].status = "updating"
        logger.info(f"Triggering model update on {device_id} to {new_model}")
        
        # In real implementation, this would send update command to device
        # For now, simulate update
        await asyncio.sleep(2)
        
        self.devices[device_id].current_model = new_model
        self.devices[device_id].status = "online"
        
        return True
    
    def add_update_callback(self, callback):
        """Add callback for device updates"""
        self.update_callbacks.append(callback)

# Global device manager instance
device_manager = DeviceManager() 